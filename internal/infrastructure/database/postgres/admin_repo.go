package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/domain/repository"
	"wanpey/core/internal/infrastructure/database"
	"wanpey/core/internal/infrastructure/database/postgres/gen"
	"wanpey/core/pkg/apperror"
)

type adminRepo struct {
	db database.SQLDB
	q  *gen.Queries
}

func NewAdminRepo(db database.SQLDB) repository.AdminRepository {
	return &adminRepo{db: db, q: gen.New(db)}
}

func (r *adminRepo) queries(ctx context.Context) *gen.Queries {
	if tx := database.TxFromContext(ctx); tx != nil {
		return r.q.WithTx(tx)
	}
	return r.q
}

func (r *adminRepo) Save(ctx context.Context, a *entity.Admin) error {
	row, err := r.queries(ctx).InsertAdmin(ctx, gen.InsertAdminParams{
		Username:     a.Username,
		PasswordHash: a.PasswordHash,
		Role:         string(a.Role),
	})
	if err != nil {
		return fmt.Errorf("insert admin: %w", err)
	}
	a.ID = row.ID
	a.CreatedAt = row.CreatedAt
	a.UpdatedAt = row.UpdatedAt
	return nil
}

func (r *adminRepo) FindByID(ctx context.Context, id string) (*entity.Admin, error) {
	row, err := r.queries(ctx).GetAdminByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NotFound("admin %s not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get admin by id: %w", err)
	}
	return toEntityAdmin(row), nil
}

func (r *adminRepo) FindByUsername(ctx context.Context, username string) (*entity.Admin, error) {
	row, err := r.queries(ctx).GetAdminByUsername(ctx, username)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NotFound("admin %s not found", username)
	}
	if err != nil {
		return nil, fmt.Errorf("get admin by username: %w", err)
	}
	return toEntityAdmin(row), nil
}

func toEntityAdmin(a gen.Admin) *entity.Admin {
	return &entity.Admin{
		ID:           a.ID,
		Username:     a.Username,
		PasswordHash: a.PasswordHash,
		Role:         entity.AdminRole(a.Role),
		CreatedAt:    a.CreatedAt,
		UpdatedAt:    a.UpdatedAt,
	}
}
