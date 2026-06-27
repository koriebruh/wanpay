package impl

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"

	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/domain/repository"
	"wanpey/core/internal/infrastructure/config"
	"wanpey/core/internal/usecase"
	"wanpey/core/pkg/apperror"
	"wanpey/core/pkg/jwtutil"
)

type adminUsecase struct {
	adminRepo    repository.AdminRepository
	merchantRepo repository.MerchantRepository
	merchantUC   usecase.MerchantUsecase
	cfg          config.AdminConfig
}

func NewAdminUsecase(
	adminRepo repository.AdminRepository,
	merchantRepo repository.MerchantRepository,
	merchantUC usecase.MerchantUsecase,
	cfg config.AdminConfig,
) usecase.AdminUsecase {
	return &adminUsecase{
		adminRepo:    adminRepo,
		merchantRepo: merchantRepo,
		merchantUC:   merchantUC,
		cfg:          cfg,
	}
}

func (u *adminUsecase) Login(ctx context.Context, input usecase.AdminLoginInput) (*usecase.AdminLoginOutput, error) {
	admin, err := u.adminRepo.FindByUsername(ctx, input.Username)
	if err != nil {
		// Never distinguish "no such user" from "wrong password" — prevents enumeration.
		return nil, apperror.Unauthorized("invalid credentials")
	}
	if bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(input.Password)) != nil {
		return nil, apperror.Unauthorized("invalid credentials")
	}
	return u.issueTokens(admin), nil
}

func (u *adminUsecase) RefreshToken(ctx context.Context, refreshToken string) (*usecase.AdminLoginOutput, error) {
	claims, err := jwtutil.Verify(u.cfg.JWTSecret, refreshToken)
	if err != nil {
		return nil, apperror.Unauthorized("invalid refresh token")
	}
	if claims.Type != jwtutil.TokenTypeRefresh {
		return nil, apperror.Unauthorized("not a refresh token")
	}
	admin, err := u.adminRepo.FindByID(ctx, claims.Sub)
	if err != nil {
		return nil, apperror.Unauthorized("admin no longer exists")
	}
	return u.issueTokens(admin), nil
}

func (u *adminUsecase) issueTokens(admin *entity.Admin) *usecase.AdminLoginOutput {
	now := time.Now()
	accessExp := now.Add(time.Duration(u.cfg.AccessTokenTTLHours) * time.Hour)
	refreshExp := now.Add(time.Duration(u.cfg.RefreshTokenTTLHours) * time.Hour)

	base := jwtutil.Claims{Sub: admin.ID, Username: admin.Username, Role: string(admin.Role)}
	access := base
	access.Type = jwtutil.TokenTypeAccess
	access.Exp = accessExp.Unix()
	refresh := base
	refresh.Type = jwtutil.TokenTypeRefresh
	refresh.Exp = refreshExp.Unix()

	return &usecase.AdminLoginOutput{
		AccessToken:  jwtutil.Generate(u.cfg.JWTSecret, access),
		RefreshToken: jwtutil.Generate(u.cfg.JWTSecret, refresh),
		ExpiresAt:    accessExp.Unix(),
	}
}

func (u *adminUsecase) CreateAdmin(ctx context.Context, input usecase.CreateAdminInput) (*usecase.AdminOutput, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	a := &entity.Admin{
		Username:     input.Username,
		PasswordHash: string(hash),
		Role:         input.Role,
	}
	if err := u.adminRepo.Save(ctx, a); err != nil {
		return nil, fmt.Errorf("create admin: %w", err)
	}
	return &usecase.AdminOutput{
		ID:        a.ID,
		Username:  a.Username,
		Role:      a.Role,
		CreatedAt: a.CreatedAt,
	}, nil
}

func (u *adminUsecase) CreateMerchant(ctx context.Context, input usecase.CreateMerchantInput) (*usecase.CreateMerchantOutput, error) {
	return u.merchantUC.Create(ctx, input)
}

func (u *adminUsecase) ApproveMerchant(ctx context.Context, merchantID string) error {
	return u.merchantUC.Activate(ctx, merchantID)
}

func (u *adminUsecase) SuspendMerchant(ctx context.Context, merchantID string) error {
	return u.merchantUC.Suspend(ctx, merchantID)
}

func (u *adminUsecase) SetMerchantFee(ctx context.Context, input usecase.SetMerchantFeeInput) error {
	m, err := u.merchantRepo.FindByID(ctx, input.MerchantID)
	if err != nil {
		return err
	}
	m.FeeConfig = input.FeeConfig
	if err := u.merchantRepo.Update(ctx, m); err != nil {
		return fmt.Errorf("update merchant fee: %w", err)
	}
	return nil
}
