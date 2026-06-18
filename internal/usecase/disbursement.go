package usecase

import (
	"context"
	"time"

	"wanpey/core/internal/domain/entity"
)

type DisburseInput struct {
	MerchantID    string                `json:"-"`
	Provider      entity.Provider       `json:"provider"        validate:"required,oneof=xendit doku"`
	BankCode      entity.BankCode       `json:"bank_code"       validate:"required,oneof=BCA BNI BRI BSI MANDIRI PERMATA CIMB"`
	AccountNumber string                `json:"account_number"  validate:"required,min=5,max=20"`
	AccountName   string                `json:"account_name"    validate:"required,max=100"`
	Amount        int64                 `json:"amount"          validate:"required,gt=0"`
	Currency      entity.Currency       `json:"currency"        validate:"required,oneof=IDR"`
	Description   string                `json:"description"     validate:"max=255"`
}

type DisbursementOutput struct {
	ID            string                    `json:"id"`
	ExternalID    string                    `json:"external_id,omitempty"`
	Provider      entity.Provider           `json:"provider"`
	Status        entity.DisbursementStatus `json:"status"`
	BankCode      entity.BankCode           `json:"bank_code"`
	AccountNumber string                    `json:"account_number"`
	AccountName   string                    `json:"account_name"`
	Amount        int64                     `json:"amount"`
	FeeAmount     int64                     `json:"fee_amount"`
	Currency      entity.Currency           `json:"currency"`
	Description   string                    `json:"description"`
	FailureReason string                    `json:"failure_reason,omitempty"`
	CompletedAt   *time.Time                `json:"completed_at,omitempty"`
	CreatedAt     time.Time                 `json:"created_at"`
}

type DisbursementUsecase interface {
	Disburse(ctx context.Context, input DisburseInput) (*DisbursementOutput, error)
	GetDisbursement(ctx context.Context, merchantID, disbursementID string) (*DisbursementOutput, error)
	HandleDisbursementCallback(ctx context.Context, provider entity.Provider, headers map[string]string, body []byte) error
}
