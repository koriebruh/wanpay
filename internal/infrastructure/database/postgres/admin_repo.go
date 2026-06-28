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
		Email:        a.Email,
		PasswordHash: a.PasswordHash,
		Role:         string(a.Role),
	})
	if err != nil {
		return fmt.Errorf("insert admin: %w", err)
	}
	a.ID = row.ID
	a.IsActive = row.IsActive
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

func (r *adminRepo) FindByEmail(ctx context.Context, email string) (*entity.Admin, error) {
	row, err := r.queries(ctx).GetAdminByEmail(ctx, email)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NotFound("admin not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get admin by email: %w", err)
	}
	return toEntityAdmin(row), nil
}

func (r *adminRepo) UpdateLastLogin(ctx context.Context, id string) error {
	if err := r.queries(ctx).UpdateAdminLastLogin(ctx, id); err != nil {
		return fmt.Errorf("update admin last_login_at: %w", err)
	}
	return nil
}

func (r *adminRepo) Update(ctx context.Context, a *entity.Admin) error {
	row, err := r.queries(ctx).UpdateAdmin(ctx, gen.UpdateAdminParams{
		ID:       a.ID,
		Role:     string(a.Role),
		IsActive: a.IsActive,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return apperror.NotFound("admin %s not found", a.ID)
	}
	if err != nil {
		return fmt.Errorf("update admin: %w", err)
	}
	a.UpdatedAt = row.UpdatedAt
	return nil
}

func (r *adminRepo) UpdatePassword(ctx context.Context, id, hash string) error {
	if err := r.queries(ctx).UpdateAdminPassword(ctx, gen.UpdateAdminPasswordParams{ID: id, PasswordHash: hash}); err != nil {
		return fmt.Errorf("update admin password: %w", err)
	}
	return nil
}

func (r *adminRepo) List(ctx context.Context, page, limit int) ([]*entity.Admin, int64, error) {
	page, limit = normalizePage(page, limit)

	total, err := r.queries(ctx).CountAdmins(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count admins: %w", err)
	}
	if total == 0 {
		return []*entity.Admin{}, 0, nil
	}

	rows, err := r.queries(ctx).ListAdmins(ctx, gen.ListAdminsParams{
		Limit:  int32(limit),                 //nolint:gosec // capped by normalizePage
		Offset: int32((page - 1) * limit),    //nolint:gosec
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list admins: %w", err)
	}

	result := make([]*entity.Admin, len(rows))
	for i, row := range rows {
		result[i] = toEntityAdmin(row)
	}
	return result, total, nil
}

func toEntityAdmin(a gen.Admin) *entity.Admin {
	return &entity.Admin{
		ID:           a.ID,
		Email:        a.Email,
		PasswordHash: a.PasswordHash,
		Role:         entity.AdminRole(a.Role),
		IsActive:     a.IsActive,
		LastLoginAt:  nullTime(a.LastLoginAt),
		CreatedAt:    a.CreatedAt,
		UpdatedAt:    a.UpdatedAt,
	}
}
