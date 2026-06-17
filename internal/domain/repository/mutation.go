package repository

import (
	"context"
	"time"

	"wanpey/core/internal/domain/entity"
)

type ListMutationFilter struct {
	MerchantID string
	Type       *entity.MutationType
	StartDate  *time.Time
	EndDate    *time.Time
	Page       int
	Limit      int
}

// MutationRepository is the persistence port for Mutation ledger records.
// Mutations are immutable — only Save and List are supported, no Update or Delete.
type MutationRepository interface {
	Save(ctx context.Context, mutation *entity.Mutation) error
	FindByID(ctx context.Context, id string) (*entity.Mutation, error)
	// FindByReferenceID looks up a mutation by referenceID + referenceType.
	// Both are required because the unique constraint is (reference_id, reference_type).
	FindByReferenceID(ctx context.Context, referenceID string, refType entity.MutationReferenceType) (*entity.Mutation, error)
	List(ctx context.Context, filter ListMutationFilter) ([]*entity.Mutation, int64, error)

	// GetBalance returns the current net balance for a merchant in IDR.
	// Calculated as: sum(cash_in amounts) - sum(cash_out amounts) across all mutations.
	// This is the source of truth for merchant balance — not any provider's settlement balance.
	GetBalance(ctx context.Context, merchantID string) (int64, error)
}
