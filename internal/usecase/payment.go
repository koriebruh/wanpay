package usecase

import (
	"context"
	"time"

	"wanpey/core/internal/domain/entity"
)

// --- Input DTOs ---

type CreateVAInput struct {
	MerchantID    string
	Provider      entity.Provider
	BankCode      entity.BankCode
	Amount        int64
	Currency      entity.Currency
	CustomerName  string
	CustomerEmail string
	CustomerPhone string
	Description   string
	ExpiryAt      time.Time
}

type CreateQRISInput struct {
	MerchantID    string
	Provider      entity.Provider
	Amount        int64
	Currency      entity.Currency
	CustomerName  string
	CustomerEmail string
	CustomerPhone string
	Description   string
	ExpiryAt      time.Time
}

// --- Output DTO ---

type PaymentOutput struct {
	ID            string
	ExternalID    string
	Method        entity.PaymentMethod
	Provider      entity.Provider
	Status        entity.PaymentStatus
	Amount        int64 // gross amount paid by customer
	FeeAmount     int64 // fee charged to merchant (0 while pending)
	Currency      entity.Currency
	CustomerName  string
	CustomerEmail string
	// VA-specific
	VANumber string
	BankCode entity.BankCode
	// QRIS-specific
	QRString   string
	QRImageURL string
	ExpiryAt    time.Time
	PaidAt      *time.Time
	CancelledAt *time.Time
	CreatedAt   time.Time
}

// --- Interface ---

// PaymentUsecase handles the lifecycle of a cash-in payment (VA & QRIS).
//
// Each mutating method (CreateVA, CreateQRIS, CancelPayment) runs a single DB transaction
// that includes an outbox insert for reliable webhook delivery to the merchant.
// Do not wrap these calls in an outer transaction.
type PaymentUsecase interface {
	CreateVA(ctx context.Context, input CreateVAInput) (*PaymentOutput, error)
	CreateQRIS(ctx context.Context, input CreateQRISInput) (*PaymentOutput, error)
	GetPayment(ctx context.Context, merchantID, paymentID string) (*PaymentOutput, error)
	CancelPayment(ctx context.Context, merchantID, paymentID string) error

	// HandleWebhook is called by the webhook HTTP handler for each provider's callback.
	// It verifies the provider signature, updates payment status, inserts a Mutation on
	// paid status, and queues an outbox event to notify the merchant — all in one transaction.
	HandleWebhook(ctx context.Context, provider entity.Provider, headers map[string]string, body []byte) error
}
