package usecase

import (
	"context"
	"time"

	"wanpey/core/internal/domain/entity"
)

// --- Input DTOs ---

type DisburseInput struct {
	MerchantID    string
	Provider      entity.Provider
	BankCode      entity.BankCode
	AccountNumber string
	AccountName   string
	Amount        int64
	Currency      entity.Currency
	Description   string
}

// --- Output DTO ---

type DisbursementOutput struct {
	ID            string
	ExternalID    string
	Provider      entity.Provider
	Status        entity.DisbursementStatus
	BankCode      entity.BankCode
	AccountNumber string
	AccountName   string
	Amount        int64 // gross disbursement amount
	FeeAmount     int64 // fee charged to merchant for this disbursement
	Currency      entity.Currency
	Description   string
	FailureReason string
	CompletedAt   *time.Time
	CreatedAt     time.Time
}

// --- Interface ---

// DisbursementUsecase handles cash-out transfers to bank accounts.
//
// Disburse creates a Disbursement record and calls the provider.
// On completion (via callback or polling), a Mutation is inserted as an immutable ledger record.
type DisbursementUsecase interface {
	Disburse(ctx context.Context, input DisburseInput) (*DisbursementOutput, error)
	GetDisbursement(ctx context.Context, merchantID, disbursementID string) (*DisbursementOutput, error)

	// HandleDisbursementCallback is called by the webhook HTTP handler for provider disbursement callbacks.
	// It verifies the provider signature, updates disbursement status, inserts a Mutation on
	// completion, and queues an outbox event — all in one transaction.
	HandleDisbursementCallback(ctx context.Context, provider entity.Provider, headers map[string]string, body []byte) error
}
