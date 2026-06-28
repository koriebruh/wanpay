package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sqlc-dev/pqtype"

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
		Va:           vaJSON,
		Qris:         qrisJSON,
		Disbursement: disbJSON,
		UpdatedBy:    f.UpdatedBy,
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

func (r *FeeRepo) CreateHoliday(ctx context.Context, h *entity.FeeHoliday) error {
	sJSON, err := json.Marshal(h.Surcharge)
	if err != nil {
		return fmt.Errorf("marshal surcharge: %w", err)
	}
	row, err := r.queries(ctx).InsertFeeHoliday(ctx, gen.InsertFeeHolidayParams{
		Name:      h.Name,
		Date:      h.Date,
		Type:      string(h.Type),
		Surcharge: sJSON,
		IsActive:  h.IsActive,
		CreatedBy: h.CreatedBy,
	})
	if err != nil {
		return fmt.Errorf("insert holiday: %w", err)
	}
	h.ID = row.ID
	h.CreatedAt = row.CreatedAt
	h.UpdatedAt = row.UpdatedAt
	return nil
}

func (r *FeeRepo) GetHolidayByDate(ctx context.Context, date time.Time) (*entity.FeeHoliday, error) {
	row, err := r.queries(ctx).GetFeeHolidayByDate(ctx, date)
	if err != nil {
		return nil, fmt.Errorf("get holiday by date: %w", err)
	}
	return toEntityHoliday(row)
}

func (r *FeeRepo) GetHolidayByID(ctx context.Context, id string) (*entity.FeeHoliday, error) {
	row, err := r.queries(ctx).GetFeeHolidayByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get holiday by id: %w", err)
	}
	return toEntityHoliday(row)
}

func (r *FeeRepo) UpdateHoliday(ctx context.Context, h *entity.FeeHoliday) error {
	sJSON, err := json.Marshal(h.Surcharge)
	if err != nil {
		return fmt.Errorf("marshal surcharge: %w", err)
	}
	row, err := r.queries(ctx).UpdateFeeHoliday(ctx, gen.UpdateFeeHolidayParams{
		ID:        h.ID,
		Name:      h.Name,
		Date:      h.Date,
		Type:      string(h.Type),
		Surcharge: sJSON,
		IsActive:  h.IsActive,
		UpdatedBy: h.UpdatedBy,
	})
	if err != nil {
		return fmt.Errorf("update holiday: %w", err)
	}
	h.UpdatedAt = row.UpdatedAt
	return nil
}

func (r *FeeRepo) ListHolidays(ctx context.Context, page, limit int) ([]*entity.FeeHoliday, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	total, err := r.q.CountFeeHolidays(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count holidays: %w", err)
	}
	rows, err := r.q.ListFeeHolidays(ctx, gen.ListFeeHolidaysParams{
		Limit:  int32(limit),              //nolint:gosec
		Offset: int32((page - 1) * limit), //nolint:gosec
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list holidays: %w", err)
	}
	result := make([]*entity.FeeHoliday, 0, len(rows))
	for _, row := range rows {
		h, err := toEntityHoliday(row)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, h)
	}
	return result, total, nil
}

func toEntityHoliday(r gen.FeeHoliday) (*entity.FeeHoliday, error) {
	h := &entity.FeeHoliday{
		ID:        r.ID,
		Name:      r.Name,
		Date:      r.Date,
		Type:      entity.HolidayType(r.Type),
		IsActive:  r.IsActive,
		CreatedBy: r.CreatedBy,
		UpdatedBy: r.UpdatedBy,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
	if err := json.Unmarshal(r.Surcharge, &h.Surcharge); err != nil {
		return nil, fmt.Errorf("unmarshal surcharge: %w", err)
	}
	return h, nil
}

// WriteAuditLog records a fee change. Must be called inside the same tx as the fee update.
func (r *FeeRepo) WriteAuditLog(ctx context.Context, log *entity.FeeAuditLog) error {
	newJSON, err := json.Marshal(log.NewValue)
	if err != nil {
		return fmt.Errorf("marshal new_value: %w", err)
	}
	var oldJSON pqtype.NullRawMessage
	if log.OldValue != nil {
		b, err := json.Marshal(log.OldValue)
		if err != nil {
			return fmt.Errorf("marshal old_value: %w", err)
		}
		oldJSON = pqtype.NullRawMessage{RawMessage: b, Valid: true}
	}
	return r.queries(ctx).InsertFeeAuditLog(ctx, gen.InsertFeeAuditLogParams{
		EntityType: log.EntityType,
		EntityID:   log.EntityID,
		AdminID:    log.AdminID,
		AdminEmail: log.AdminEmail,
		OldValue:   oldJSON,
		NewValue:   newJSON,
		Reason:     log.Reason,
	})
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
