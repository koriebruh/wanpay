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

type paymentRepo struct {
	db database.SQLDB
	q  *gen.Queries
}

func NewPaymentRepo(db database.SQLDB) repository.PaymentRepository {
	return &paymentRepo{db: db, q: gen.New(db)}
}

func (r *paymentRepo) queries(ctx context.Context) *gen.Queries {
	if tx := database.TxFromContext(ctx); tx != nil {
		return r.q.WithTx(tx)
	}
	return r.q
}

func (r *paymentRepo) Save(ctx context.Context, p *entity.Payment) error {
	meta, err := json.Marshal(p.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	row, err := r.queries(ctx).InsertPayment(ctx, gen.InsertPaymentParams{
		MerchantID:    p.MerchantID,
		ExternalID:    p.ExternalID,
		Method:        string(p.Method),
		Provider:      string(p.Provider),
		Status:        string(p.Status),
		Amount:        p.Amount,
		FeeAmount:     p.FeeAmount,
		Currency:      string(p.Currency),
		Description:   p.Description,
		CustomerName:  p.CustomerName,
		CustomerEmail: p.CustomerEmail,
		CustomerPhone: p.CustomerPhone,
		VaNumber:      p.VANumber,
		BankCode:      string(p.BankCode),
		QrString:      p.QRString,
		QrImageUrl:    p.QRImageURL,
		ExpiryAt:      p.ExpiryAt,
		Metadata:      meta,
	})
	if err != nil {
		return fmt.Errorf("insert payment: %w", err)
	}
	p.ID = row.ID
	p.CreatedAt = row.CreatedAt
	p.UpdatedAt = row.UpdatedAt
	return nil
}

func (r *paymentRepo) FindByID(ctx context.Context, id string) (*entity.Payment, error) {
	row, err := r.queries(ctx).GetPaymentByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NotFound("payment %s not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get payment by id: %w", err)
	}
	return toEntityPayment(row), nil
}

func (r *paymentRepo) FindByExternalID(ctx context.Context, provider entity.Provider, externalID string) (*entity.Payment, error) {
	row, err := r.queries(ctx).GetPaymentByExternalID(ctx, gen.GetPaymentByExternalIDParams{
		Provider:   string(provider),
		ExternalID: externalID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NotFound("payment with external_id %s not found", externalID)
	}
	if err != nil {
		return nil, fmt.Errorf("get payment by external_id: %w", err)
	}
	return toEntityPayment(row), nil
}

func (r *paymentRepo) Update(ctx context.Context, p *entity.Payment) error {
	_, err := r.queries(ctx).UpdatePaymentStatus(ctx, gen.UpdatePaymentStatusParams{
		ID:          p.ID,
		Status:      string(p.Status),
		FeeAmount:   p.FeeAmount,
		PaidAt:      toNullTime(p.PaidAt),
		FailedAt:    toNullTime(p.FailedAt),
		CancelledAt: toNullTime(p.CancelledAt),
	})
	if err != nil {
		return fmt.Errorf("update payment status: %w", err)
	}
	return nil
}

func (r *paymentRepo) List(ctx context.Context, f repository.ListPaymentFilter) ([]*entity.Payment, int64, error) {
	q := database.QuerierFromContext(ctx, r.db)

	conds := []string{"merchant_id = $1"}
	args := []any{f.MerchantID}
	idx := 2

	if f.Status != nil {
		conds = append(conds, fmt.Sprintf("status = $%d", idx))
		args = append(args, string(*f.Status))
		idx++
	}
	if f.Method != nil {
		conds = append(conds, fmt.Sprintf("method = $%d", idx))
		args = append(args, string(*f.Method))
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
	if err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM payments"+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count payments: %w", err)
	}
	if total == 0 {
		return []*entity.Payment{}, 0, nil
	}

	page, limit := normalizePage(f.Page, f.Limit)
	listArgs := append(args, int32(limit), int32((page-1)*limit)) //nolint:gocritic,gosec // limit capped at maxLimit=100, never overflows int32
	query := fmt.Sprintf(
		"SELECT id, merchant_id, external_id, method, provider, status, amount, fee_amount, currency, description, customer_name, customer_email, customer_phone, va_number, bank_code, qr_string, qr_image_url, expiry_at, paid_at, failed_at, cancelled_at, created_at, updated_at, metadata FROM payments%s ORDER BY created_at DESC LIMIT $%d OFFSET $%d",
		where, idx, idx+1,
	)

	rows, err := q.QueryContext(ctx, query, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list payments: %w", err)
	}
	defer rows.Close()

	var result []*entity.Payment
	for rows.Next() {
		var p gen.Payment
		if err := rows.Scan(
			&p.ID, &p.MerchantID, &p.ExternalID, &p.Method, &p.Provider, &p.Status,
			&p.Amount, &p.FeeAmount, &p.Currency, &p.Description,
			&p.CustomerName, &p.CustomerEmail, &p.CustomerPhone,
			&p.VaNumber, &p.BankCode, &p.QrString, &p.QrImageUrl,
			&p.ExpiryAt, &p.PaidAt, &p.FailedAt, &p.CancelledAt,
			&p.CreatedAt, &p.UpdatedAt, &p.Metadata,
		); err != nil {
			return nil, 0, fmt.Errorf("scan payment: %w", err)
		}
		result = append(result, toEntityPayment(p))
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows error: %w", err)
	}
	return result, total, nil
}
