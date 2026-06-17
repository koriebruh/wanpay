package repository

import (
	"context"
	"time"

	"wanpey/core/internal/domain/entity"
)

type ListPaymentFilter struct {
	MerchantID string
	Status     *entity.PaymentStatus
	Method     *entity.PaymentMethod
	Provider   *entity.Provider
	StartDate  *time.Time
	EndDate    *time.Time
	Page       int
	Limit      int
}

// PaymentRepository is the persistence port for Payment entities.
// Implementations must guarantee that Save and Update are called within
// the same DB transaction as any outbox insert for atomicity.
type PaymentRepository interface {
	Save(ctx context.Context, payment *entity.Payment) error
	FindByID(ctx context.Context, id string) (*entity.Payment, error)
	// FindByExternalID looks up a payment by provider + externalID.
	// Both are required because the unique constraint is (provider, external_id).
	FindByExternalID(ctx context.Context, provider entity.Provider, externalID string) (*entity.Payment, error)
	Update(ctx context.Context, payment *entity.Payment) error
	List(ctx context.Context, filter ListPaymentFilter) ([]*entity.Payment, int64, error)
}
