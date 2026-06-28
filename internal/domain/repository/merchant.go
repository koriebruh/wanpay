package repository

import (
	"context"

	"wanpey/core/internal/domain/entity"
)

// ListMerchantFilter filters merchant list queries.
// If Status is empty, all statuses are returned. If Search is non-empty,
// name and email are searched (case-insensitive prefix).
type ListMerchantFilter struct {
	Status string
	Search string
	Page   int
	Limit  int
}

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
	FindBankAccountByID(ctx context.Context, accountID string) (*entity.MerchantBankAccount, error)
	FindPrimaryBankAccount(ctx context.Context, merchantID string) (*entity.MerchantBankAccount, error)
	UpdateBankAccount(ctx context.Context, account *entity.MerchantBankAccount) error
	// UnsetPrimaryBankAccounts clears the is_primary flag on all accounts for a merchant.
	// Call before setting a new primary to maintain the single-primary invariant.
	UnsetPrimaryBankAccounts(ctx context.Context, merchantID string) error
	DeleteBankAccount(ctx context.Context, accountID string) error
	CountBankAccounts(ctx context.Context, merchantID string) (int, error)
	// List returns a paginated list of merchants with optional filters.
	// If filter.Status is empty all statuses are included.
	// If filter.Search is non-empty it matches name or email (ILIKE prefix).
	List(ctx context.Context, filter ListMerchantFilter) ([]*entity.Merchant, int64, error)
	// SoftDelete sets deleted_at on the merchant record (never hard-deletes).
	SoftDelete(ctx context.Context, merchantID string) error
}
