package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/domain/repository"
	"wanpey/core/internal/infrastructure/database"
	"wanpey/core/internal/infrastructure/database/postgres/gen"
	"wanpey/core/pkg/apperror"
)

type merchantRepo struct {
	db database.SQLDB
	q  *gen.Queries
}

func NewMerchantRepo(db database.SQLDB) repository.MerchantRepository {
	return &merchantRepo{db: db, q: gen.New(db)}
}

func (r *merchantRepo) queries(ctx context.Context) *gen.Queries {
	if tx := database.TxFromContext(ctx); tx != nil {
		return r.q.WithTx(tx)
	}
	return r.q
}

func (r *merchantRepo) Save(ctx context.Context, m *entity.Merchant) error {
	feeJSON, err := json.Marshal(m.FeeConfig)
	if err != nil {
		return fmt.Errorf("marshal fee_config: %w", err)
	}
	row, err := r.queries(ctx).InsertMerchant(ctx, gen.InsertMerchantParams{
		Name:          m.Name,
		Email:         m.Email,
		Phone:         m.Phone,
		Status:        string(m.Status),
		ApiKey:        m.APIKey,
		WebhookUrl:    m.WebhookURL,
		WebhookSecret: m.WebhookSecret,
		FeeConfig:     feeJSON,
	})
	if err != nil {
		return fmt.Errorf("insert merchant: %w", err)
	}
	m.ID = row.ID
	m.CreatedAt = row.CreatedAt
	m.UpdatedAt = row.UpdatedAt
	return nil
}

func (r *merchantRepo) FindByID(ctx context.Context, id string) (*entity.Merchant, error) {
	row, err := r.queries(ctx).GetMerchantByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NotFound("merchant %s not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get merchant by id: %w", err)
	}
	return toEntityMerchant(row), nil
}

func (r *merchantRepo) FindByAPIKey(ctx context.Context, hashedKey string) (*entity.Merchant, error) {
	row, err := r.queries(ctx).GetMerchantByAPIKey(ctx, hashedKey)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NotFound("merchant not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get merchant by api_key: %w", err)
	}
	return toEntityMerchant(row), nil
}

func (r *merchantRepo) FindByEmail(ctx context.Context, email string) (*entity.Merchant, error) {
	row, err := r.queries(ctx).GetMerchantByEmail(ctx, email)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NotFound("merchant with email %s not found", email)
	}
	if err != nil {
		return nil, fmt.Errorf("get merchant by email: %w", err)
	}
	return toEntityMerchant(row), nil
}

func (r *merchantRepo) Update(ctx context.Context, m *entity.Merchant) error {
	feeJSON, err := json.Marshal(m.FeeConfig)
	if err != nil {
		return fmt.Errorf("marshal fee_config: %w", err)
	}
	row, err := r.queries(ctx).UpdateMerchant(ctx, gen.UpdateMerchantParams{
		ID:                m.ID,
		Name:              m.Name,
		Email:             m.Email,
		Phone:             m.Phone,
		Status:            string(m.Status),
		ApiKey:            m.APIKey,
		WebhookUrl:        m.WebhookURL,
		WebhookSecret:     m.WebhookSecret,
		FeeConfig:         feeJSON,
		DailyCashoutLimit: m.DailyCashoutLimit,
	})
	if err != nil {
		return fmt.Errorf("update merchant: %w", err)
	}
	m.UpdatedAt = row.UpdatedAt
	return nil
}

func (r *merchantRepo) SaveBankAccount(ctx context.Context, a *entity.MerchantBankAccount) error {
	row, err := r.queries(ctx).InsertBankAccount(ctx, gen.InsertBankAccountParams{
		MerchantID:    a.MerchantID,
		BankCode:      string(a.BankCode),
		AccountNumber: a.AccountNumber,
		AccountName:   a.AccountName,
		IsPrimary:     a.IsPrimary,
		IsVerified:    a.IsVerified,
	})
	if err != nil {
		return fmt.Errorf("insert bank account: %w", err)
	}
	a.ID = row.ID
	a.CreatedAt = row.CreatedAt
	a.UpdatedAt = row.UpdatedAt
	return nil
}

func (r *merchantRepo) FindBankAccountsByMerchantID(ctx context.Context, merchantID string) ([]*entity.MerchantBankAccount, error) {
	rows, err := r.queries(ctx).ListBankAccountsByMerchant(ctx, merchantID)
	if err != nil {
		return nil, fmt.Errorf("list bank accounts: %w", err)
	}
	result := make([]*entity.MerchantBankAccount, len(rows))
	for i, b := range rows {
		result[i] = toEntityBankAccount(b)
	}
	return result, nil
}

func (r *merchantRepo) FindBankAccountByID(ctx context.Context, accountID string) (*entity.MerchantBankAccount, error) {
	row, err := r.queries(ctx).GetBankAccountByID(ctx, accountID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NotFound("bank account %s not found", accountID)
	}
	if err != nil {
		return nil, fmt.Errorf("get bank account by id: %w", err)
	}
	return toEntityBankAccount(row), nil
}

func (r *merchantRepo) UnsetPrimaryBankAccounts(ctx context.Context, merchantID string) error {
	if err := r.queries(ctx).UnsetPrimaryBankAccounts(ctx, merchantID); err != nil {
		return fmt.Errorf("unset primary bank accounts: %w", err)
	}
	return nil
}

func (r *merchantRepo) FindPrimaryBankAccount(ctx context.Context, merchantID string) (*entity.MerchantBankAccount, error) {
	row, err := r.queries(ctx).GetPrimaryBankAccount(ctx, merchantID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NotFound("no primary bank account for merchant %s", merchantID)
	}
	if err != nil {
		return nil, fmt.Errorf("get primary bank account: %w", err)
	}
	return toEntityBankAccount(row), nil
}

func (r *merchantRepo) UpdateBankAccount(ctx context.Context, a *entity.MerchantBankAccount) error {
	row, err := r.queries(ctx).UpdateBankAccount(ctx, gen.UpdateBankAccountParams{
		ID:            a.ID,
		BankCode:      string(a.BankCode),
		AccountNumber: a.AccountNumber,
		AccountName:   a.AccountName,
		IsPrimary:     a.IsPrimary,
		IsVerified:    a.IsVerified,
	})
	if err != nil {
		return fmt.Errorf("update bank account: %w", err)
	}
	a.UpdatedAt = row.UpdatedAt
	return nil
}

func (r *merchantRepo) DeleteBankAccount(ctx context.Context, accountID string) error {
	if err := r.queries(ctx).DeleteBankAccount(ctx, accountID); err != nil {
		return fmt.Errorf("delete bank account: %w", err)
	}
	return nil
}

func (r *merchantRepo) CountBankAccounts(ctx context.Context, merchantID string) (int, error) {
	n, err := r.queries(ctx).CountBankAccounts(ctx, merchantID)
	if err != nil {
		return 0, fmt.Errorf("count bank accounts: %w", err)
	}
	return int(n), nil
}

func (r *merchantRepo) List(ctx context.Context, f repository.ListMerchantFilter) ([]*entity.Merchant, int64, error) {
	q := database.QuerierFromContext(ctx, r.db)

	var conds []string
	var args []any
	idx := 1

	if f.Status != "" {
		conds = append(conds, fmt.Sprintf("status = $%d", idx))
		args = append(args, f.Status)
		idx++
	}
	if f.Search != "" {
		conds = append(conds, fmt.Sprintf("(name ILIKE $%d OR email ILIKE $%d)", idx, idx))
		args = append(args, f.Search+"%")
		idx++
	}

	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	var total int64
	if err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM merchants"+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count merchants: %w", err)
	}
	if total == 0 {
		return []*entity.Merchant{}, 0, nil
	}

	page, limit := normalizePage(f.Page, f.Limit)
	listArgs := append(args, int32(limit), int32((page-1)*limit)) //nolint:gocritic,gosec
	query := fmt.Sprintf(
		"SELECT id, name, email, phone, status, api_key, webhook_url, webhook_secret, fee_config, daily_cashout_limit, deleted_at, created_at, updated_at FROM merchants%s ORDER BY created_at DESC LIMIT $%d OFFSET $%d",
		where, idx, idx+1,
	)

	rows, err := q.QueryContext(ctx, query, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list merchants: %w", err)
	}
	defer rows.Close()

	var result []*entity.Merchant
	for rows.Next() {
		var m gen.Merchant
		if err := rows.Scan(
			&m.ID, &m.Name, &m.Email, &m.Phone, &m.Status,
			&m.ApiKey, &m.WebhookUrl, &m.WebhookSecret, &m.FeeConfig,
			&m.DailyCashoutLimit, &m.DeletedAt, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan merchant: %w", err)
		}
		result = append(result, toEntityMerchant(m))
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows error: %w", err)
	}
	return result, total, nil
}

func (r *merchantRepo) SoftDelete(ctx context.Context, merchantID string) error {
	if err := r.queries(ctx).SoftDeleteMerchant(ctx, merchantID); err != nil {
		return fmt.Errorf("soft delete merchant: %w", err)
	}
	return nil
}
