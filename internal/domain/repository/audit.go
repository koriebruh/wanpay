package repository

import (
	"context"

	"wanpey/core/internal/domain/entity"
)

// AuditRepository is the persistence port for payment audit trail records.
// Records are append-only — Save is the only mutating operation.
// Implementations must never expose an Update or Delete method.
type AuditRepository interface {
	Save(ctx context.Context, audit *entity.PaymentAudit) error
	FindByPaymentID(ctx context.Context, paymentID string) ([]*entity.PaymentAudit, error)
}
