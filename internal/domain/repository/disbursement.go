package repository

import (
	"context"
	"time"

	"wanpey/core/internal/domain/entity"
)

type ListDisbursementFilter struct {
	MerchantID string
	Status     *entity.DisbursementStatus
	Provider   *entity.Provider
	StartDate  *time.Time
	EndDate    *time.Time
	Page       int
	Limit      int
}

// DisbursementRepository is the persistence port for Disbursement entities.
type DisbursementRepository interface {
	Save(ctx context.Context, disbursement *entity.Disbursement) error
	FindByID(ctx context.Context, id string) (*entity.Disbursement, error)
	FindByExternalID(ctx context.Context, externalID string) (*entity.Disbursement, error)
	Update(ctx context.Context, disbursement *entity.Disbursement) error
	List(ctx context.Context, filter ListDisbursementFilter) ([]*entity.Disbursement, int64, error)
}
