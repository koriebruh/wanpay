package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/infrastructure/database"
	"wanpey/core/internal/infrastructure/database/postgres/gen"
)

// FeeRepo implements repository.FeeRepository.
// Both tables have exactly one row seeded by migration.
type FeeRepo struct {
	db database.SQLDB
	q  *gen.Queries
}

func NewFeeRepo(db database.SQLDB) *FeeRepo {
	return &FeeRepo{db: db, q: gen.New(db)}
}

func (r *FeeRepo) queries(ctx context.Context) *gen.Queries {
	if tx := database.TxFromContext(ctx); tx != nil {
		return r.q.WithTx(tx)
	}
	return r.q
}

func (r *FeeRepo) GetDefault(ctx context.Context) (*entity.FeeDefault, error) {
	row, err := r.queries(ctx).GetFeeDefault(ctx)
	if err != nil {
		return nil, fmt.Errorf("get fee default: %w", err)
	}
	return toEntityFeeDefault(row)
}

func (r *FeeRepo) UpdateDefault(ctx context.Context, f *entity.FeeDefault) error {
	vaJSON, err := json.Marshal(f.VA)
	if err != nil {
		return fmt.Errorf("marshal va fee: %w", err)
	}
	qrisJSON, err := json.Marshal(f.QRIS)
	if err != nil {
		return fmt.Errorf("marshal qris fee: %w", err)
	}
	disbJSON, err := json.Marshal(f.Disbursement)
	if err != nil {
		return fmt.Errorf("marshal disbursement fee: %w", err)
	}
	row, err := r.queries(ctx).UpdateFeeDefault(ctx, gen.UpdateFeeDefaultParams{
		Va:          vaJSON,
		Qris:        qrisJSON,
		Disbursement: disbJSON,
		UpdatedBy:   f.UpdatedBy,
	})
	if err != nil {
		return fmt.Errorf("update fee default: %w", err)
	}
	f.UpdatedAt = row.UpdatedAt
	return nil
}

func (r *FeeRepo) GetMargin(ctx context.Context) (*entity.PlatformMargin, error) {
	row, err := r.queries(ctx).GetPlatformMargin(ctx)
	if err != nil {
		return nil, fmt.Errorf("get platform margin: %w", err)
	}
	return toEntityPlatformMargin(row)
}

func (r *FeeRepo) UpdateMargin(ctx context.Context, m *entity.PlatformMargin) error {
	vaJSON, err := json.Marshal(m.VA)
	if err != nil {
		return fmt.Errorf("marshal va margin: %w", err)
	}
	qrisJSON, err := json.Marshal(m.QRIS)
	if err != nil {
		return fmt.Errorf("marshal qris margin: %w", err)
	}
	disbJSON, err := json.Marshal(m.Disbursement)
	if err != nil {
		return fmt.Errorf("marshal disbursement margin: %w", err)
	}
	row, err := r.queries(ctx).UpdatePlatformMargin(ctx, gen.UpdatePlatformMarginParams{
		Enabled:      m.Enabled,
		Va:           vaJSON,
		Qris:         qrisJSON,
		Disbursement: disbJSON,
		UpdatedBy:    m.UpdatedBy,
	})
	if err != nil {
		return fmt.Errorf("update platform margin: %w", err)
	}
	m.UpdatedAt = row.UpdatedAt
	return nil
}

func toEntityFeeDefault(r gen.FeeDefault) (*entity.FeeDefault, error) {
	f := &entity.FeeDefault{
		ID:        r.ID,
		UpdatedBy: r.UpdatedBy,
		UpdatedAt: r.UpdatedAt,
		CreatedAt: r.CreatedAt,
	}
	if err := json.Unmarshal(r.Va, &f.VA); err != nil {
		return nil, fmt.Errorf("unmarshal va fee: %w", err)
	}
	if err := json.Unmarshal(r.Qris, &f.QRIS); err != nil {
		return nil, fmt.Errorf("unmarshal qris fee: %w", err)
	}
	if err := json.Unmarshal(r.Disbursement, &f.Disbursement); err != nil {
		return nil, fmt.Errorf("unmarshal disbursement fee: %w", err)
	}
	return f, nil
}

func toEntityPlatformMargin(r gen.PlatformMargin) (*entity.PlatformMargin, error) {
	m := &entity.PlatformMargin{
		ID:        r.ID,
		Enabled:   r.Enabled,
		UpdatedBy: r.UpdatedBy,
		UpdatedAt: r.UpdatedAt,
		CreatedAt: r.CreatedAt,
	}
	if err := json.Unmarshal(r.Va, &m.VA); err != nil {
		return nil, fmt.Errorf("unmarshal va margin: %w", err)
	}
	if err := json.Unmarshal(r.Qris, &m.QRIS); err != nil {
		return nil, fmt.Errorf("unmarshal qris margin: %w", err)
	}
	if err := json.Unmarshal(r.Disbursement, &m.Disbursement); err != nil {
		return nil, fmt.Errorf("unmarshal disbursement margin: %w", err)
	}
	return m, nil
}
