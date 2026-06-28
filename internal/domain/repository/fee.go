package repository

import (
	"context"
	"time"

	"wanpey/core/internal/domain/entity"
)

// FeeRepository manages the singleton global fee defaults, platform margin, holidays, and audit log.
type FeeRepository interface {
	GetDefault(ctx context.Context) (*entity.FeeDefault, error)
	UpdateDefault(ctx context.Context, f *entity.FeeDefault) error
	GetMargin(ctx context.Context) (*entity.PlatformMargin, error)
	UpdateMargin(ctx context.Context, m *entity.PlatformMargin) error

	// Holidays
	CreateHoliday(ctx context.Context, h *entity.FeeHoliday) error
	GetHolidayByDate(ctx context.Context, date time.Time) (*entity.FeeHoliday, error)
	GetHolidayByID(ctx context.Context, id string) (*entity.FeeHoliday, error)
	UpdateHoliday(ctx context.Context, h *entity.FeeHoliday) error
	ListHolidays(ctx context.Context, page, limit int) ([]*entity.FeeHoliday, int64, error)

	// WriteAuditLog appends an immutable fee-change record. Call inside the same tx as the update.
	WriteAuditLog(ctx context.Context, log *entity.FeeAuditLog) error
}
