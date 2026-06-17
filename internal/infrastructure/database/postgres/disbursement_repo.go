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

type disbursementRepo struct {
	db database.SQLDB
	q  *gen.Queries
}

func NewDisbursementRepo(db database.SQLDB) repository.DisbursementRepository {
	return &disbursementRepo{db: db, q: gen.New(db)}
}

func (r *disbursementRepo) queries(ctx context.Context) *gen.Queries {
	if tx := database.TxFromContext(ctx); tx != nil {
		return r.q.WithTx(tx)
	}
	return r.q
}

func (r *disbursementRepo) Save(ctx context.Context, d *entity.Disbursement) error {
	row, err := r.queries(ctx).InsertDisbursement(ctx, gen.InsertDisbursementParams{
		MerchantID:    d.MerchantID,
		ExternalID:    d.ExternalID,
		Provider:      string(d.Provider),
		Status:        string(d.Status),
		BankCode:      string(d.BankCode),
		AccountNumber: d.AccountNumber,
		AccountName:   d.AccountName,
		Amount:        d.Amount,
		FeeAmount:     d.FeeAmount,
		Currency:      string(d.Currency),
		Description:   d.Description,
	})
	if err != nil {
		return fmt.Errorf("insert disbursement: %w", err)
	}
	d.ID = row.ID
	d.CreatedAt = row.CreatedAt
	d.UpdatedAt = row.UpdatedAt
	return nil
}

func (r *disbursementRepo) FindByID(ctx context.Context, id string) (*entity.Disbursement, error) {
	row, err := r.queries(ctx).GetDisbursementByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NotFound("disbursement %s not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get disbursement by id: %w", err)
	}
	return toEntityDisbursement(row), nil
}

func (r *disbursementRepo) FindByExternalID(ctx context.Context, provider entity.Provider, externalID string) (*entity.Disbursement, error) {
	row, err := r.queries(ctx).GetDisbursementByExternalID(ctx, gen.GetDisbursementByExternalIDParams{
		Provider:   string(provider),
		ExternalID: externalID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NotFound("disbursement with external_id %s not found", externalID)
	}
	if err != nil {
		return nil, fmt.Errorf("get disbursement by external_id: %w", err)
	}
	return toEntityDisbursement(row), nil
}

func (r *disbursementRepo) Update(ctx context.Context, d *entity.Disbursement) error {
	_, err := r.queries(ctx).UpdateDisbursementStatus(ctx, gen.UpdateDisbursementStatusParams{
		ID:            d.ID,
		Status:        string(d.Status),
		ExternalID:    d.ExternalID,
		FailureReason: d.FailureReason,
		CompletedAt:   toNullTime(d.CompletedAt),
		FailedAt:      toNullTime(d.FailedAt),
	})
	if err != nil {
		return fmt.Errorf("update disbursement: %w", err)
	}
	return nil
}

func (r *disbursementRepo) List(ctx context.Context, f repository.ListDisbursementFilter) ([]*entity.Disbursement, int64, error) {
	q := database.QuerierFromContext(ctx, r.db)

	conds := []string{"merchant_id = $1"}
	args := []any{f.MerchantID}
	idx := 2

	if f.Status != nil {
		conds = append(conds, fmt.Sprintf("status = $%d", idx))
		args = append(args, string(*f.Status))
		idx++
	}
	if f.Provider != nil {
		conds = append(conds, fmt.Sprintf("provider = $%d", idx))
		args = append(args, string(*f.Provider))
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
	if err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM disbursements"+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count disbursements: %w", err)
	}
	if total == 0 {
		return []*entity.Disbursement{}, 0, nil
	}

	page, limit := normalizePage(f.Page, f.Limit)
	listArgs := append(args, int32(limit), int32((page-1)*limit)) //nolint:gocritic,gosec // limit capped at maxLimit=100, never overflows int32
	query := fmt.Sprintf(
		"SELECT id, merchant_id, external_id, provider, status, bank_code, account_number, account_name, amount, fee_amount, currency, description, failure_reason, completed_at, failed_at, created_at, updated_at FROM disbursements%s ORDER BY created_at DESC LIMIT $%d OFFSET $%d",
		where, idx, idx+1,
	)

	rows, err := q.QueryContext(ctx, query, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list disbursements: %w", err)
	}
	defer rows.Close()

	var result []*entity.Disbursement
	for rows.Next() {
		var d gen.Disbursement
		if err := rows.Scan(
			&d.ID, &d.MerchantID, &d.ExternalID, &d.Provider, &d.Status,
			&d.BankCode, &d.AccountNumber, &d.AccountName,
			&d.Amount, &d.FeeAmount, &d.Currency, &d.Description, &d.FailureReason,
			&d.CompletedAt, &d.FailedAt, &d.CreatedAt, &d.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan disbursement: %w", err)
		}
		result = append(result, toEntityDisbursement(d))
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows error: %w", err)
	}
	return result, total, nil
}
