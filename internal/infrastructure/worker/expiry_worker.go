package worker

import (
	"context"
	"time"

	"go.uber.org/zap"

	"wanpey/core/internal/infrastructure/database"
)

const expiryPollInterval = time.Minute

// ExpiryWorker polls every minute and marks payments that have passed
// their expiry_at as expired. Many providers never send an "expired"
// webhook — only "paid" — so without this, pending payments accumulate
// forever and balance calculations become inaccurate.
type ExpiryWorker struct {
	db  database.SQLDB
	log *zap.Logger
}

func NewExpiryWorker(db database.SQLDB, log *zap.Logger) *ExpiryWorker {
	return &ExpiryWorker{db: db, log: log}
}

func (w *ExpiryWorker) Run(ctx context.Context) {
	w.log.Info("expiry worker started")
	ticker := time.NewTicker(expiryPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.log.Info("expiry worker stopped")
			return
		case <-ticker.C:
			w.expirePayments(ctx)
		}
	}
}

func (w *ExpiryWorker) expirePayments(ctx context.Context) {
	const q = `
		WITH expired AS (
			UPDATE payments
			SET status     = 'expired',
			    updated_at = NOW()
			WHERE status   = 'pending'
			  AND expiry_at < NOW()
			RETURNING id, merchant_id
		)
		INSERT INTO payment_audits (id, payment_id, event_type, old_status, new_status, actor, created_at)
		SELECT gen_random_uuid(), id, 'PAYMENT_EXPIRED', 'pending', 'expired', 'system:expiry_worker', NOW()
		FROM expired`

	result, err := w.db.ExecContext(ctx, q)
	if err != nil {
		w.log.Error("expiry worker: failed to expire payments", zap.Error(err))
		return
	}
	n, _ := result.RowsAffected()
	if n > 0 {
		w.log.Info("expiry worker: payments expired", zap.Int64("count", n))
	}
}
