package usecase

import (
	"context"
	"time"

	"wanpey/core/internal/domain/entity"
)

// --- Input DTO ---

type ListMutationsInput struct {
	MerchantID string
	Type       *entity.MutationType // nil = all types
	StartDate  *time.Time
	EndDate    *time.Time
	Page       int
	Limit      int
}

// --- Output DTOs ---

type MutationOutput struct {
	ID          string
	ReferenceID string // PaymentID or DisbursementID
	Type        entity.MutationType
	Amount      int64
	Currency    entity.Currency
	Description string
	CreatedAt   time.Time
}

type MutationListOutput struct {
	Items []*MutationOutput
	Total int64
	Page  int
	Limit int
}

// --- Interface ---

// MutationUsecase exposes the immutable ledger of completed cash-in and cash-out events.
// Mutations cannot be created directly through this usecase — they are produced by
// PaymentUsecase (on paid) and DisbursementUsecase (on completed).
type MutationUsecase interface {
	ListMutations(ctx context.Context, input ListMutationsInput) (*MutationListOutput, error)
	GetMutation(ctx context.Context, merchantID, mutationID string) (*MutationOutput, error)
}
