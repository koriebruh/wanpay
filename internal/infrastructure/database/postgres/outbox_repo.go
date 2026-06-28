package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"wanpey/core/internal/infrastructure/database"
	"wanpey/core/internal/infrastructure/database/postgres/gen"
)

// OutboxRepo handles outbox event persistence for the outbox worker.
// It is not a domain repository — it is an infrastructure concern only.
type OutboxRepo struct {
	db database.SQLDB
	q  *gen.Queries
}

func NewOutboxRepo(db database.SQLDB) *OutboxRepo {
	return &OutboxRepo{db: db, q: gen.New(db)}
}

func (r *OutboxRepo) queries(ctx context.Context) *gen.Queries {
	if tx := database.TxFromContext(ctx); tx != nil {
		return r.q.WithTx(tx)
	}
	return r.q
}

// Insert adds an outbox event. Must be called inside database.RunInTx
// alongside the status update it accompanies to guarantee atomicity.
func (r *OutboxRepo) Insert(ctx context.Context, eventType, targetURL, merchantID string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal outbox payload: %w", err)
	}
	_, err = r.queries(ctx).InsertOutboxEvent(ctx, gen.InsertOutboxEventParams{
		EventType:  eventType,
		Payload:    b,
		TargetUrl:  targetURL,
		MerchantID: uuid.NullUUID{UUID: uuid.MustParse(merchantID), Valid: merchantID != ""},
	})
	if err != nil {
		return fmt.Errorf("insert outbox event: %w", err)
	}
	return nil
}

// Lease returns up to limit undelivered events and locks them for processing.
// Uses FOR UPDATE SKIP LOCKED — concurrent workers never pick the same row.
func (r *OutboxRepo) Lease(ctx context.Context, limit int) ([]gen.Outbox, error) {
	rows, err := r.q.LeaseOutboxEvents(ctx, int32(limit)) //nolint:gosec // limit is user-controlled input capped before this call
	if err != nil {
		return nil, fmt.Errorf("lease outbox events: %w", err)
	}
	return rows, nil
}

// MarkDelivered marks an event as successfully delivered.
func (r *OutboxRepo) MarkDelivered(ctx context.Context, id string) error {
	if err := r.q.MarkOutboxDelivered(ctx, id); err != nil {
		return fmt.Errorf("mark outbox delivered: %w", err)
	}
	return nil
}

// ListByMerchant returns outbox events for a merchant, newest first.
func (r *OutboxRepo) ListByMerchant(ctx context.Context, merchantID string, page, limit int) ([]gen.Outbox, int64, error) {
	mid := uuid.MustParse(merchantID)
	total, err := r.q.CountOutboxByMerchant(ctx, uuid.NullUUID{UUID: mid, Valid: true})
	if err != nil {
		return nil, 0, fmt.Errorf("count outbox: %w", err)
	}
	if total == 0 {
		return nil, 0, nil
	}
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := r.q.ListOutboxByMerchant(ctx, gen.ListOutboxByMerchantParams{
		MerchantID: uuid.NullUUID{UUID: mid, Valid: true},
		Limit:      int32(limit),  //nolint:gosec
		Offset:     int32((page - 1) * limit), //nolint:gosec
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list outbox by merchant: %w", err)
	}
	return rows, total, nil
}

// MarkFailed increments the attempt counter and schedules the next retry.
// When max_attempts is reached, failed_at is set and the event stops being leased.
func (r *OutboxRepo) MarkFailed(ctx context.Context, id string, nextRetry time.Time) error {
	if err := r.q.MarkOutboxFailed(ctx, gen.MarkOutboxFailedParams{
		ID:          id,
		NextRetryAt: nextRetry,
	}); err != nil {
		return fmt.Errorf("mark outbox failed: %w", err)
	}
	return nil
}
