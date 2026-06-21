package repository

import (
	"context"

	"wanpey/core/internal/domain/entity"
)

// ProviderBalanceRepository manages the platform's balance records at each provider.
type ProviderBalanceRepository interface {
	Upsert(ctx context.Context, balance *entity.ProviderBalance) error
	FindByProvider(ctx context.Context, provider entity.Provider) (*entity.ProviderBalance, error)
	ListAll(ctx context.Context) ([]*entity.ProviderBalance, error)
}
