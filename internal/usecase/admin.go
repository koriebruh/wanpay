package usecase

import (
	"context"
	"time"

	"wanpey/core/internal/domain/entity"
)

type AdminLoginInput struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

type AdminLoginOutput struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"` // Unix timestamp
}

type CreateAdminInput struct {
	RequesterID string            `json:"-"`
	Username    string            `json:"username" validate:"required,min=3,max=50"`
	Password    string            `json:"password" validate:"required,min=8"`
	Role        entity.AdminRole  `json:"role"     validate:"required,oneof=admin super_admin"`
}

type AdminOutput struct {
	ID        string           `json:"id"`
	Username  string           `json:"username"`
	Role      entity.AdminRole `json:"role"`
	CreatedAt time.Time        `json:"created_at"`
}

type SetMerchantFeeInput struct {
	MerchantID string           `json:"-"`
	FeeConfig  entity.FeeConfig `json:"fee_config"`
}

type AdminUsecase interface {
	Login(ctx context.Context, input AdminLoginInput) (*AdminLoginOutput, error)
	RefreshToken(ctx context.Context, refreshToken string) (*AdminLoginOutput, error)
	CreateAdmin(ctx context.Context, input CreateAdminInput) (*AdminOutput, error)
	// Merchant management — admin is the only path to create/approve merchants
	// after public registration was removed.
	CreateMerchant(ctx context.Context, input CreateMerchantInput) (*CreateMerchantOutput, error)
	ApproveMerchant(ctx context.Context, merchantID string) error
	SuspendMerchant(ctx context.Context, merchantID string) error
	SetMerchantFee(ctx context.Context, input SetMerchantFeeInput) error
}
