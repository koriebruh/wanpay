package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/domain/repository"
	"wanpey/core/internal/infrastructure/database"
	"wanpey/core/internal/infrastructure/database/postgres/gen"
	"wanpey/core/pkg/apperror"
)

type mutationRepo struct {
	db database.SQLDB
	q  *gen.Queries
}

func NewMutationRepo(db database.SQLDB) repository.MutationRepository {
	return &mutationRepo{db: db, q: gen.New(db)}
}

func (r *mutationRepo) queries(ctx context.Context) *gen.Queries {
	if tx := database.TxFromContext(ctx); tx != nil {
		return r.q.WithTx(tx)
	}
	return r.q
}

func (r *mutationRepo) Save(ctx context.Context, m *entity.Mutation) error {
	row, err := r.queries(ctx).InsertMutation(ctx, gen.InsertMutationParams{
		ReferenceID:   m.ReferenceID,
		ReferenceType: string(m.ReferenceType),
		MerchantID:    m.MerchantID,
		Type:          string(m.Type),
		Amount:        m.Amount,
		FeeAmount:     m.FeeAmount,
		Currency:      string(m.Currency),
		Description:   m.Description,
	})
	if err != nil {
		return fmt.Errorf("insert mutation: %w", err)
	}
	m.ID = row.ID
	m.CreatedAt = row.CreatedAt
	return nil
}

func (r *mutationRepo) FindByID(ctx context.Context, id string) (*entity.Mutation, error) {
	row, err := r.queries(ctx).GetMutationByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NotFound("mutation %s not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get mutation by id: %w", err)
	}
	return toEntityMutation(row), nil
}

func (r *mutationRepo) FindByReferenceID(ctx context.Context, referenceID string, refType entity.MutationReferenceType) (*entity.Mutation, error) {
	row, err := r.queries(ctx).GetMutationByReferenceID(ctx, gen.GetMutationByReferenceIDParams{
		ReferenceID:   referenceID,
		ReferenceType: string(refType),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NotFound("mutation for reference %s not found", referenceID)
	}
	if err != nil {
		return nil, fmt.Errorf("get mutation by reference_id: %w", err)
	}
	return toEntityMutation(row), nil
}

func (r *mutationRepo) GetBalance(ctx context.Context, merchantID string) (int64, error) {
	balance, err := r.queries(ctx).GetMerchantBalance(ctx, merchantID)
	if err != nil {
		return 0, fmt.Errorf("get merchant balance: %w", err)
	}
	return balance, nil
}

func (r *mutationRepo) List(ctx context.Context, f repository.ListMutationFilter) ([]*entity.Mutation, int64, error) {
	q := database.QuerierFromContext(ctx, r.db)

	conds := []string{"merchant_id = $1"}
	args := []any{f.MerchantID}
	idx := 2

	if f.Type != nil {
		conds = append(conds, fmt.Sprintf("type = $%d", idx))
		args = append(args, string(*f.Type))
		idx++
	}
	if f.StartDate != nil {
		conds = append(conds, fmt.Sprintf("created_at >= $%d", idx))
		args = append(args, *f.StartDate)
		idx++
	}
	if f.EndDate != nil {
		conds = append(conds, fmt.Sprintf("created_at <= $%d", idx))
		args = append(args, *f.EndDate)
		idx++
	}

	where := " WHERE " + strings.Join(conds, " AND ")

	var total int64
	if err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM mutations"+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count mutations: %w", err)
	}
	if total == 0 {
		return []*entity.Mutation{}, 0, nil
	}

	page, limit := normalizePage(f.Page, f.Limit)
	listArgs := append(args, int32(limit), int32((page-1)*limit)) //nolint:gocritic
	query := fmt.Sprintf(
		"SELECT id, reference_id, reference_type, merchant_id, type, amount, fee_amount, currency, description, created_at FROM mutations%s ORDER BY created_at DESC LIMIT $%d OFFSET $%d",
		where, idx, idx+1,
	)

	rows, err := q.QueryContext(ctx, query, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list mutations: %w", err)
	}
	defer rows.Close()

	var result []*entity.Mutation
	for rows.Next() {
		var m gen.Mutation
		if err := rows.Scan(
			&m.ID, &m.ReferenceID, &m.ReferenceType, &m.MerchantID,
			&m.Type, &m.Amount, &m.FeeAmount, &m.Currency, &m.Description, &m.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan mutation: %w", err)
		}
		result = append(result, toEntityMutation(m))
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows error: %w", err)
	}
	return result, total, nil
}
