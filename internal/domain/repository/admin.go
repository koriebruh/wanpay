package repository

import (
	"context"

	"wanpey/core/internal/domain/entity"
)

// AdminRepository is the persistence port for Admin entities.
type AdminRepository interface {
	Save(ctx context.Context, admin *entity.Admin) error
	FindByID(ctx context.Context, id string) (*entity.Admin, error)
	FindByEmail(ctx context.Context, email string) (*entity.Admin, error)
	UpdateLastLogin(ctx context.Context, id string) error
	Update(ctx context.Context, admin *entity.Admin) error
	List(ctx context.Context, page, limit int) ([]*entity.Admin, int64, error)
}
