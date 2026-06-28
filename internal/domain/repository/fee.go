package repository

import (
	"context"

	"wanpey/core/internal/domain/entity"
)

// FeeRepository manages the singleton global fee defaults and platform margin rows.
// Both tables have exactly one row — Get returns it; Update modifies it in-place.
type FeeRepository interface {
	GetDefault(ctx context.Context) (*entity.FeeDefault, error)
	UpdateDefault(ctx context.Context, f *entity.FeeDefault) error
	GetMargin(ctx context.Context) (*entity.PlatformMargin, error)
	UpdateMargin(ctx context.Context, m *entity.PlatformMargin) error
}
