package worker

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"wanpey/core/internal/infrastructure/database"
)

const (
	outboxPollInterval  = 5 * time.Second
	outboxHTTPTimeout   = 10 * time.Second
	outboxClaimLease    = 60 * time.Second
	outboxMaxErrLen     = 500
	outboxMaxConcurrent = 5  // max parallel webhook deliveries per batch
)

type outboxRow struct {
	ID        string
	EventType string
	Payload   json.RawMessage
	TargetURL string
	Attempt   int
	Max       int
}

type OutboxWorker struct {
	db         database.SQLDB
	httpClient *http.Client
	log        *zap.Logger
}

func NewOutboxWorker(db database.SQLDB, log *zap.Logger) *OutboxWorker {
	return &OutboxWorker{
		db:  db,
		log: log,
		httpClient: &http.Client{
			Timeout: outboxHTTPTimeout,
			// Disable redirects — a merchant callback URL must not redirect to internal services.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (w *OutboxWorker) Run(ctx context.Context) {
	w.log.Info("outbox worker started")
	ticker := time.NewTicker(outboxPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.log.Info("outbox worker stopped")
			return
		case <-ticker.C:
			w.processBatch(ctx)
		}
	}
}

func (w *OutboxWorker) processBatch(ctx context.Context) {
	rows, err := w.fetchAndClaim(ctx)
	if err != nil {
		w.log.Error("outbox: fetch pending failed", zap.Error(err))
		return
	}
	if len(rows) == 0 {
		return
	}

	sem := make(chan struct{}, outboxMaxConcurrent)
	var wg sync.WaitGroup
	for _, row := range rows {
		wg.Add(1)
		sem <- struct{}{}
		go func(r outboxRow) {
			defer wg.Done()
			defer func() { <-sem }()
			w.deliver(ctx, r)
		}(row)
	}
	wg.Wait()
}

// fetchAndClaim atomically claims up to 10 pending rows by advancing next_retry_at by
// outboxClaimLease. This prevents double-delivery without holding an open transaction
// across the slow HTTP delivery step. If the worker dies mid-delivery, rows become
// re-eligible after the lease expires.
func (w *OutboxWorker) fetchAndClaim(ctx context.Context) ([]outboxRow, error) {
	const q = `
		UPDATE outbox
		SET next_retry_at = NOW() + $1::interval
		WHERE id IN (
			SELECT id FROM outbox
			WHERE delivered_at IS NULL
			  AND attempt_count < max_attempts
			  AND next_retry_at <= NOW()
			ORDER BY next_retry_at
			LIMIT 10
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, event_type, payload, target_url, attempt_count, max_attempts`

	lease := fmt.Sprintf("%d seconds", int(outboxClaimLease.Seconds()))
	rows, err := w.db.QueryContext(ctx, q, lease)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []outboxRow
	for rows.Next() {
		var r outboxRow
		if err := rows.Scan(&r.ID, &r.EventType, &r.Payload, &r.TargetURL, &r.Attempt, &r.Max); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (w *OutboxWorker) deliver(ctx context.Context, row outboxRow) {
	log := w.log.With(
		zap.String("outbox_id", row.ID),
		zap.String("event_type", row.EventType),
		zap.String("target_url", row.TargetURL),
		zap.Int("attempt", row.Attempt+1),
	)

	// dbCtx is intentionally decoupled from ctx so that DB status updates
	// succeed even when the parent ctx is cancelled (e.g. during graceful shutdown).
	// A 5-second deadline is enough for a simple UPDATE.
	dbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := validateWebhookURL(row.TargetURL); err != nil {
		log.Error("outbox: invalid target_url, marking failed", zap.Error(err))
		if markErr := w.markFailed(dbCtx, row.ID, err.Error()); markErr != nil {
			log.Error("outbox: mark failed error", zap.Error(markErr))
		}
		return
	}

	err := w.post(ctx, row.TargetURL, row.Payload)
	if err == nil {
		if updateErr := w.markDelivered(dbCtx, row.ID); updateErr != nil {
			log.Error("outbox: mark delivered failed", zap.Error(updateErr))
		} else {
			log.Info("outbox: webhook delivered")
		}
		return
	}

	log.Warn("outbox: delivery failed", zap.Error(err))

	nextAttempt := row.Attempt + 1
	if nextAttempt >= row.Max {
		if markErr := w.markFailed(dbCtx, row.ID, truncate(err.Error(), outboxMaxErrLen)); markErr != nil {
			log.Error("outbox: mark failed error", zap.Error(markErr))
		}
		log.Error("outbox: max attempts reached, giving up", zap.Int("max_attempts", row.Max))
		return
	}

	nextRetry := backoff(nextAttempt)
	if schedErr := w.scheduleRetry(dbCtx, row.ID, nextAttempt, truncate(err.Error(), outboxMaxErrLen), nextRetry); schedErr != nil {
		log.Error("outbox: schedule retry failed", zap.Error(schedErr))
	}
}

func (w *OutboxWorker) post(ctx context.Context, rawURL string, payload json.RawMessage) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("non-2xx response: %d", resp.StatusCode)
	}
	return nil
}

func (w *OutboxWorker) markDelivered(ctx context.Context, id string) error {
	_, err := w.db.ExecContext(ctx,
		`UPDATE outbox SET delivered_at = NOW() WHERE id = $1`, id)
	return err
}

func (w *OutboxWorker) markFailed(ctx context.Context, id, lastErr string) error {
	_, err := w.db.ExecContext(ctx,
		`UPDATE outbox SET attempt_count = max_attempts, last_error = $2, failed_at = NOW() WHERE id = $1`,
		id, lastErr)
	return err
}

func (w *OutboxWorker) scheduleRetry(ctx context.Context, id string, nextAttempt int, lastErr string, delay time.Duration) error {
	_, err := w.db.ExecContext(ctx,
		`UPDATE outbox SET attempt_count = $2, last_error = $3, next_retry_at = NOW() + $4::interval WHERE id = $1`,
		id, nextAttempt, lastErr, fmt.Sprintf("%d seconds", int(delay.Seconds())))
	return err
}

// backoff returns exponential delay for attempt n: 1→10s, 2→40s, 3→90s, 4→160s, 5→250s.
func backoff(attempt int) time.Duration {
	return time.Duration(math.Pow(float64(attempt), 2)) * 10 * time.Second
}

// validateWebhookURL rejects non-HTTP schemes and private/loopback targets.
func validateWebhookURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("url scheme must be http or https, got %q", u.Scheme)
	}
	if isPrivateHost(u.Hostname()) {
		return fmt.Errorf("url targets a private or loopback address: %s", u.Hostname())
	}
	return nil
}

// isPrivateHost returns true for localhost, loopback IPs, and RFC-1918 private ranges.
// Note: DNS rebinding is not covered here — use a custom dialer for production hardening.
func isPrivateHost(host string) bool {
	if strings.ToLower(host) == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max]
	}
	return s
}

// InsertOutbox writes an outbox entry inside an existing DB transaction.
// Must be called in the same tx as the payment status update to guarantee atomicity.
func InsertOutbox(ctx context.Context, tx *sql.Tx, eventType, targetURL string, payload any) error {
	if err := validateWebhookURL(targetURL); err != nil {
		return fmt.Errorf("insecure target_url rejected: %w", err)
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal outbox payload: %w", err)
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO outbox (event_type, payload, target_url) VALUES ($1, $2, $3)`,
		eventType, b, targetURL)
	return err
}
