package usecase

import (
	"context"
	"time"

	"wanpey/core/internal/domain/entity"
)

type ListMutationsInput struct {
	MerchantID string               `json:"-"`
	Type       *entity.MutationType `json:"type,omitempty"`
	StartDate  *time.Time           `json:"start_date,omitempty"`
	EndDate    *time.Time           `json:"end_date,omitempty"`
	Page       int                  `json:"page"`
	Limit      int                  `json:"limit"`
}

type MutationOutput struct {
	ID            string                       `json:"id"`
	ReferenceID   string                       `json:"reference_id"`
	ReferenceType entity.MutationReferenceType `json:"reference_type"`
	Type          entity.MutationType          `json:"type"`
	Amount        int64                        `json:"amount"`
	FeeAmount     int64                        `json:"fee_amount"`
	NetAmount     int64                        `json:"net_amount"`
	Currency      entity.Currency              `json:"currency"`
	Description   string                       `json:"description"`
	CreatedAt     time.Time                    `json:"created_at"`
}

type MutationListOutput struct {
	Items []*MutationOutput `json:"items"`
	Total int64             `json:"total"`
	Page  int               `json:"page"`
	Limit int               `json:"limit"`
}

type MutationUsecase interface {
	ListMutations(ctx context.Context, input ListMutationsInput) (*MutationListOutput, error)
	GetMutation(ctx context.Context, merchantID, mutationID string) (*MutationOutput, error)
	GetBalance(ctx context.Context, merchantID string) (int64, error)
}
