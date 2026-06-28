package impl

import (
	"context"

	"wanpey/core/internal/infrastructure/database/postgres/gen"
)

// outboxPort is the subset of postgres.OutboxRepo used by usecases.
// Defined here so tests can stub it without a real DB connection.
type outboxPort interface {
	Insert(ctx context.Context, eventType, targetURL, merchantID string, payload any) error
	ListByMerchant(ctx context.Context, merchantID string, page, limit int) ([]gen.Outbox, int64, error)
}
