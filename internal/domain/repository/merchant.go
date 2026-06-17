package repository

import (
	"context"

	"wanpey/core/internal/domain/entity"
)

// MerchantRepository is the persistence port for Merchant and MerchantBankAccount entities.
type MerchantRepository interface {
	Save(ctx context.Context, merchant *entity.Merchant) error
	FindByID(ctx context.Context, id string) (*entity.Merchant, error)
	// FindByAPIKey looks up a merchant by the SHA256 hash of their API key.
	// Used in the authentication middleware on every inbound request.
	FindByAPIKey(ctx context.Context, hashedKey string) (*entity.Merchant, error)
	// FindByEmail looks up a merchant by email address.
	// Used during onboarding to prevent duplicate registrations.
	FindByEmail(ctx context.Context, email string) (*entity.Merchant, error)
	Update(ctx context.Context, merchant *entity.Merchant) error

	// SaveBankAccount persists a new bank account.
	// Callers must enforce the MaxBankAccounts limit before calling.
	SaveBankAccount(ctx context.Context, account *entity.MerchantBankAccount) error
	FindBankAccountsByMerchantID(ctx context.Context, merchantID string) ([]*entity.MerchantBankAccount, error)
	FindPrimaryBankAccount(ctx context.Context, merchantID string) (*entity.MerchantBankAccount, error)
	UpdateBankAccount(ctx context.Context, account *entity.MerchantBankAccount) error
	DeleteBankAccount(ctx context.Context, accountID string) error
	CountBankAccounts(ctx context.Context, merchantID string) (int, error)
}
