package gateway

import (
	"context"
	"time"

	"wanpey/core/internal/domain/entity"
)

// --- Cash In (VA & QRIS) ---

type CreateVARequest struct {
	ExternalID    string
	BankCode      entity.BankCode
	Amount        int64
	Currency      entity.Currency
	CustomerName  string
	CustomerEmail string
	CustomerPhone string
	Description   string
	ExpiryAt      time.Time
}

type CreateVAResponse struct {
	ExternalID string
	VANumber   string
	BankCode   entity.BankCode
	Amount     int64
	ExpiryAt   time.Time
}

type CreateQRISRequest struct {
	ExternalID    string
	Amount        int64
	Currency      entity.Currency
	CustomerName  string
	CustomerEmail string
	CustomerPhone string
	Description   string
	ExpiryAt      time.Time
}

type CreateQRISResponse struct {
	ExternalID string
	QRString   string // raw QR string for client-side rendering
	QRImageURL string // hosted image URL — may be empty depending on provider
	Amount     int64
	ExpiryAt   time.Time
}

type WebhookEvent struct {
	ExternalID string
	Status     entity.PaymentStatus
	PaidAt     *time.Time
	Amount     int64
	RawPayload []byte // original body preserved for payment_audits
}

// PaymentGateway is the port that every cash-in provider adapter must implement.
// Implementations live in internal/infrastructure/provider/{midtrans,xendit,doku}.
// Each implementation wraps its calls with a circuit breaker (see infrastructure/provider/circuit_breaker.go).
type PaymentGateway interface {
	CreateVA(ctx context.Context, req CreateVARequest) (*CreateVAResponse, error)
	CreateQRIS(ctx context.Context, req CreateQRISRequest) (*CreateQRISResponse, error)

	// CancelPayment sends a cancellation request to the provider for a pending payment.
	// Not all providers support this; implementations may return ErrNotSupported.
	CancelPayment(ctx context.Context, externalID string) error

	// GetStatus polls the provider for the current payment status.
	// Used as a fallback when a webhook was not received within the expected window.
	GetStatus(ctx context.Context, externalID string) (entity.PaymentStatus, error)

	// ParseWebhook verifies the provider's signature and returns a normalized event.
	// Returns an error if the signature is invalid — caller must respond 401 to the provider.
	ParseWebhook(ctx context.Context, headers map[string]string, body []byte) (*WebhookEvent, error)

	// SupportedMethods returns which payment methods this provider supports.
	// Used by the router to select a capable provider when the merchant does not specify one.
	SupportedMethods() []entity.PaymentMethod

	ProviderName() entity.Provider
}

// --- Cash Out (Disbursement) ---

type DisburseRequest struct {
	ExternalID    string
	BankCode      entity.BankCode
	AccountNumber string
	AccountName   string
	Amount        int64
	Currency      entity.Currency
	Description   string
}

type DisburseResponse struct {
	ExternalID string
	Status     entity.DisbursementStatus
	Amount     int64
}

// DisbursementWebhookEvent is a normalized disbursement status callback from a provider.
type DisbursementWebhookEvent struct {
	ExternalID    string
	Status        entity.DisbursementStatus
	FailureReason string
	Amount        int64
	RawPayload    []byte // original body preserved for audit
}

// DisbursementGateway is the port for cash-out provider adapters.
// Not all providers support disbursement (e.g., Midtrans does not) —
// only implement this interface for providers that do (Xendit, DOKU).
type DisbursementGateway interface {
	Disburse(ctx context.Context, req DisburseRequest) (*DisburseResponse, error)

	// GetDisbursementStatus polls the provider for the current disbursement status.
	GetDisbursementStatus(ctx context.Context, externalID string) (*DisburseResponse, error)

	// ParseDisbursementWebhook verifies the provider's signature and returns a normalized event.
	// Returns an error if the signature is invalid — caller must respond 401 to the provider.
	ParseDisbursementWebhook(ctx context.Context, headers map[string]string, body []byte) (*DisbursementWebhookEvent, error)

	ProviderName() entity.Provider
}
