package repository

import (
	"context"

	"wanpey/core/internal/domain/entity"
)

// AdminRepository is the persistence port for Admin entities.
type AdminRepository interface {
	Save(ctx context.Context, admin *entity.Admin) error
	FindByID(ctx context.Context, id string) (*entity.Admin, error)
	FindByUsername(ctx context.Context, username string) (*entity.Admin, error)
}
