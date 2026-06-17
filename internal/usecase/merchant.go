package usecase

import (
	"context"
	"time"

	"wanpey/core/internal/domain/entity"
)

// --- Input DTOs ---

type CreateMerchantInput struct {
	Name       string
	Email      string
	Phone      string
	FeeConfig  entity.FeeConfig
	WebhookURL string
}

type UpdateMerchantInput struct {
	MerchantID string
	Name       string
	Email      string
	Phone      string
	WebhookURL string
	FeeConfig  entity.FeeConfig
}

type AddBankAccountInput struct {
	MerchantID    string
	BankCode      entity.BankCode
	AccountNumber string
	AccountName   string
	SetAsPrimary  bool
}

// --- Output DTOs ---

type CreateMerchantOutput struct {
	ID            string
	Name          string
	Status        entity.MerchantStatus
	APIKey        string // raw key — shown once, never retrievable again
	WebhookSecret string // raw secret — shown once, never retrievable again
	CreatedAt     time.Time
}

type MerchantOutput struct {
	ID         string
	Name       string
	Email      string
	Phone      string
	Status     entity.MerchantStatus
	FeeConfig  entity.FeeConfig
	WebhookURL string
	Balance    int64 // IDR, live balance from mutation ledger
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type BankAccountOutput struct {
	ID            string
	BankCode      entity.BankCode
	AccountNumber string
	AccountName   string
	IsPrimary     bool
	CreatedAt     time.Time
}

// --- Interface ---

// MerchantUsecase manages merchant onboarding, credentials, and bank accounts.
type MerchantUsecase interface {
	Create(ctx context.Context, input CreateMerchantInput) (*CreateMerchantOutput, error)
	GetMerchant(ctx context.Context, id string) (*MerchantOutput, error)
	Update(ctx context.Context, input UpdateMerchantInput) (*MerchantOutput, error)
	Suspend(ctx context.Context, merchantID string) error
	Activate(ctx context.Context, merchantID string) error

	// RegenerateAPIKey invalidates the current key immediately and returns the new raw key.
	// New key format: wpay_live_<32 random chars> | wpay_test_<32 random chars>.
	// Caller must securely deliver the new key to the merchant — it cannot be retrieved again.
	RegenerateAPIKey(ctx context.Context, merchantID string) (rawKey string, err error)

	// Bank account management — max entity.MaxBankAccounts accounts per merchant.
	AddBankAccount(ctx context.Context, input AddBankAccountInput) (*BankAccountOutput, error)
	ListBankAccounts(ctx context.Context, merchantID string) ([]*BankAccountOutput, error)
	RemoveBankAccount(ctx context.Context, merchantID, accountID string) error
	SetPrimaryBankAccount(ctx context.Context, merchantID, accountID string) error
}
