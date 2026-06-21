package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/domain/repository"
	"wanpey/core/internal/infrastructure/database"
	"wanpey/core/internal/infrastructure/database/postgres/gen"
	"wanpey/core/pkg/apperror"
)

type providerBalanceRepo struct {
	db database.SQLDB
	q  *gen.Queries
}

func NewProviderBalanceRepo(db database.SQLDB) repository.ProviderBalanceRepository {
	return &providerBalanceRepo{db: db, q: gen.New(db)}
}

func (r *providerBalanceRepo) queries(ctx context.Context) *gen.Queries {
	if tx := database.TxFromContext(ctx); tx != nil {
		return r.q.WithTx(tx)
	}
	return r.q
}

func (r *providerBalanceRepo) Upsert(ctx context.Context, b *entity.ProviderBalance) error {
	row, err := r.queries(ctx).UpsertProviderBalance(ctx, gen.UpsertProviderBalanceParams{
		Provider:         string(b.Provider),
		BalanceIdr:       b.BalanceIDR,
		LastReconciledAt: toNullTime(b.LastReconciledAt),
		Note:             b.Note,
	})
	if err != nil {
		return fmt.Errorf("upsert provider balance: %w", err)
	}
	b.ID = row.ID
	b.CreatedAt = row.CreatedAt
	b.UpdatedAt = row.UpdatedAt
	return nil
}

func (r *providerBalanceRepo) FindByProvider(ctx context.Context, provider entity.Provider) (*entity.ProviderBalance, error) {
	row, err := r.queries(ctx).GetProviderBalance(ctx, string(provider))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NotFound("provider balance for %s not found", provider)
	}
	if err != nil {
		return nil, fmt.Errorf("get provider balance: %w", err)
	}
	return toEntityProviderBalance(row), nil
}

func (r *providerBalanceRepo) ListAll(ctx context.Context) ([]*entity.ProviderBalance, error) {
	rows, err := r.queries(ctx).ListProviderBalances(ctx)
	if err != nil {
		return nil, fmt.Errorf("list provider balances: %w", err)
	}
	result := make([]*entity.ProviderBalance, len(rows))
	for i, row := range rows {
		result[i] = toEntityProviderBalance(row)
	}
	return result, nil
}
