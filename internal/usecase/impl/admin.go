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
	adminRepo           repository.AdminRepository
	merchantRepo        repository.MerchantRepository
	merchantUC          usecase.MerchantUsecase
	paymentRepo         repository.PaymentRepository
	disbursementRepo    repository.DisbursementRepository
	mutationRepo        repository.MutationRepository
	providerBalanceRepo repository.ProviderBalanceRepository
	feeRepo             repository.FeeRepository
	cfg                 config.AdminConfig
}

func NewAdminUsecase(
	adminRepo repository.AdminRepository,
	merchantRepo repository.MerchantRepository,
	merchantUC usecase.MerchantUsecase,
	paymentRepo repository.PaymentRepository,
	disbursementRepo repository.DisbursementRepository,
	mutationRepo repository.MutationRepository,
	providerBalanceRepo repository.ProviderBalanceRepository,
	feeRepo repository.FeeRepository,
	cfg config.AdminConfig,
) usecase.AdminUsecase {
	return &adminUsecase{
		adminRepo:           adminRepo,
		merchantRepo:        merchantRepo,
		merchantUC:          merchantUC,
		paymentRepo:         paymentRepo,
		disbursementRepo:    disbursementRepo,
		mutationRepo:        mutationRepo,
		providerBalanceRepo: providerBalanceRepo,
		feeRepo:             feeRepo,
		cfg:                 cfg,
	}
}

// ── Auth ──────────────────────────────────────────────────────────────────────

func (u *adminUsecase) Login(ctx context.Context, input usecase.AdminLoginInput) (*usecase.AdminTokenOutput, error) {
	admin, err := u.adminRepo.FindByEmail(ctx, input.Email)
	if err != nil {
		// Never distinguish "no such user" from "wrong password" — prevents enumeration.
		return nil, apperror.Unauthorized("invalid credentials")
	}
	if !admin.IsActive {
		return nil, apperror.Unauthorized("invalid credentials")
	}
	if bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(input.Password)) != nil {
		return nil, apperror.Unauthorized("invalid credentials")
	}
	// Update last login asynchronously — don't fail the login if this errors.
	_ = u.adminRepo.UpdateLastLogin(ctx, admin.ID) //nolint:errcheck
	return u.issueTokens(admin), nil
}

func (u *adminUsecase) RefreshToken(ctx context.Context, refreshToken string) (*usecase.AdminTokenOutput, error) {
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
	if !admin.IsActive {
		return nil, apperror.Unauthorized("account is deactivated")
	}
	return u.issueTokens(admin), nil
}

func (u *adminUsecase) issueTokens(admin *entity.Admin) *usecase.AdminTokenOutput {
	now := time.Now()
	accessExp := now.Add(time.Duration(u.cfg.AccessTokenTTLHours) * time.Hour)
	refreshExp := now.Add(time.Duration(u.cfg.RefreshTokenTTLHours) * time.Hour)

	base := jwtutil.Claims{Sub: admin.ID, Email: admin.Email, Role: string(admin.Role)}
	access := base
	access.Type = jwtutil.TokenTypeAccess
	access.Exp = accessExp.Unix()
	refresh := base
	refresh.Type = jwtutil.TokenTypeRefresh
	refresh.Exp = refreshExp.Unix()

	return &usecase.AdminTokenOutput{
		AccessToken:  jwtutil.Generate(u.cfg.JWTSecret, access),
		RefreshToken: jwtutil.Generate(u.cfg.JWTSecret, refresh),
		ExpiresAt:    accessExp,
	}
}

// ── Merchant management ───────────────────────────────────────────────────────

func (u *adminUsecase) CreateMerchant(ctx context.Context, input usecase.CreateMerchantInput) (*usecase.CreateMerchantOutput, error) {
	return u.merchantUC.Create(ctx, input)
}

func (u *adminUsecase) ListMerchants(ctx context.Context, filter usecase.AdminListMerchantsFilter) (*usecase.MerchantListOutput, error) {
	merchants, total, err := u.merchantRepo.List(ctx, repository.ListMerchantFilter{
		Status: filter.Status,
		Search: filter.Search,
		Page:   filter.Page,
		Limit:  filter.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list merchants: %w", err)
	}
	items := make([]*usecase.MerchantOutput, len(merchants))
	for i, m := range merchants {
		items[i] = toMerchantOutput(m, 0)
	}
	page, limit := normalizePagination(filter.Page, filter.Limit)
	return &usecase.MerchantListOutput{Items: items, Total: total, Page: page, Limit: limit}, nil
}

func (u *adminUsecase) GetMerchant(ctx context.Context, merchantID string) (*usecase.MerchantOutput, error) {
	return u.merchantUC.GetMerchant(ctx, merchantID)
}

func (u *adminUsecase) ApproveMerchant(ctx context.Context, merchantID string) error {
	return u.merchantUC.Activate(ctx, merchantID)
}

func (u *adminUsecase) SuspendMerchant(ctx context.Context, merchantID string) error {
	return u.merchantUC.Suspend(ctx, merchantID)
}

func (u *adminUsecase) DeactivateMerchant(ctx context.Context, merchantID string) error {
	m, err := u.merchantRepo.FindByID(ctx, merchantID)
	if err != nil {
		return err
	}
	m.Status = entity.MerchantStatusInactive
	return u.merchantRepo.Update(ctx, m)
}

func (u *adminUsecase) DeleteMerchant(ctx context.Context, merchantID string) error {
	return u.merchantRepo.SoftDelete(ctx, merchantID)
}

func (u *adminUsecase) UpdateMerchantFee(ctx context.Context, input usecase.SetMerchantFeeInput) error {
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

func (u *adminUsecase) UpdateDailyCashoutLimit(ctx context.Context, merchantID string, limitIDR int64) error {
	m, err := u.merchantRepo.FindByID(ctx, merchantID)
	if err != nil {
		return err
	}
	m.DailyCashoutLimit = limitIDR
	if err := u.merchantRepo.Update(ctx, m); err != nil {
		return fmt.Errorf("update daily cashout limit: %w", err)
	}
	return nil
}

func (u *adminUsecase) RegenerateMerchantAPIKey(ctx context.Context, merchantID string) (string, error) {
	return u.merchantUC.RegenerateAPIKey(ctx, merchantID)
}

// ── Bank accounts ─────────────────────────────────────────────────────────────

func (u *adminUsecase) VerifyBankAccount(ctx context.Context, merchantID, accountID string) error {
	a, err := u.merchantRepo.FindBankAccountByID(ctx, accountID)
	if err != nil {
		return err
	}
	if a.MerchantID != merchantID {
		return apperror.NotFound("bank account not found")
	}
	a.IsVerified = true
	return u.merchantRepo.UpdateBankAccount(ctx, a)
}

func (u *adminUsecase) ListMerchantBankAccounts(ctx context.Context, merchantID string) ([]*usecase.BankAccountOutput, error) {
	return u.merchantUC.ListBankAccounts(ctx, merchantID)
}

// ── Visibility ────────────────────────────────────────────────────────────────

func (u *adminUsecase) ListAllPayments(ctx context.Context, filter usecase.AdminPaymentFilter) (*usecase.PaymentListOutput, error) {
	f := repository.ListPaymentFilter{
		MerchantID: filter.MerchantID,
		Page:       filter.Page,
		Limit:      filter.Limit,
	}
	if filter.Status != "" {
		s := entity.PaymentStatus(filter.Status)
		f.Status = &s
	}
	if filter.Provider != "" {
		p := entity.Provider(filter.Provider)
		f.Provider = &p
	}
	if filter.Start != "" {
		if t, err := time.Parse(time.RFC3339, filter.Start); err == nil {
			f.StartDate = &t
		}
	}
	if filter.End != "" {
		if t, err := time.Parse(time.RFC3339, filter.End); err == nil {
			f.EndDate = &t
		}
	}

	payments, total, err := u.paymentRepo.List(ctx, f)
	if err != nil {
		return nil, fmt.Errorf("list payments: %w", err)
	}
	items := make([]*usecase.PaymentOutput, len(payments))
	for i, p := range payments {
		items[i] = toPaymentOutput(p)
	}
	page, limit := normalizePagination(filter.Page, filter.Limit)
	return &usecase.PaymentListOutput{Items: items, Total: total, Page: page, Limit: limit}, nil
}

func (u *adminUsecase) GetPayment(ctx context.Context, paymentID string) (*usecase.PaymentOutput, error) {
	p, err := u.paymentRepo.FindByID(ctx, paymentID)
	if err != nil {
		return nil, err
	}
	return toPaymentOutput(p), nil
}

func (u *adminUsecase) ListAllDisbursements(ctx context.Context, filter usecase.AdminDisbursementFilter) (*usecase.DisbursementListOutput, error) {
	f := repository.ListDisbursementFilter{
		MerchantID: filter.MerchantID,
		Page:       filter.Page,
		Limit:      filter.Limit,
	}
	if filter.Status != "" {
		s := entity.DisbursementStatus(filter.Status)
		f.Status = &s
	}
	if filter.Provider != "" {
		p := entity.Provider(filter.Provider)
		f.Provider = &p
	}
	if filter.Start != "" {
		if t, err := time.Parse(time.RFC3339, filter.Start); err == nil {
			f.StartDate = &t
		}
	}
	if filter.End != "" {
		if t, err := time.Parse(time.RFC3339, filter.End); err == nil {
			f.EndDate = &t
		}
	}

	disbursements, total, err := u.disbursementRepo.List(ctx, f)
	if err != nil {
		return nil, fmt.Errorf("list disbursements: %w", err)
	}
	items := make([]*usecase.DisbursementOutput, len(disbursements))
	for i, d := range disbursements {
		items[i] = toDisbursementOutput(d)
	}
	page, limit := normalizePagination(filter.Page, filter.Limit)
	return &usecase.DisbursementListOutput{Items: items, Total: total, Page: page, Limit: limit}, nil
}

func (u *adminUsecase) GetDisbursement(ctx context.Context, disbursementID string) (*usecase.DisbursementOutput, error) {
	d, err := u.disbursementRepo.FindByID(ctx, disbursementID)
	if err != nil {
		return nil, err
	}
	return toDisbursementOutput(d), nil
}

func (u *adminUsecase) ListAllMutations(ctx context.Context, filter usecase.AdminMutationFilter) (*usecase.MutationListOutput, error) {
	f := repository.ListMutationFilter{
		MerchantID: filter.MerchantID,
		Page:       filter.Page,
		Limit:      filter.Limit,
	}
	if filter.Type != "" {
		t := entity.MutationType(filter.Type)
		f.Type = &t
	}
	if filter.Start != "" {
		if t, err := time.Parse(time.RFC3339, filter.Start); err == nil {
			f.StartDate = &t
		}
	}
	if filter.End != "" {
		if t, err := time.Parse(time.RFC3339, filter.End); err == nil {
			f.EndDate = &t
		}
	}

	mutations, total, err := u.mutationRepo.List(ctx, f)
	if err != nil {
		return nil, fmt.Errorf("list mutations: %w", err)
	}
	items := make([]*usecase.MutationOutput, len(mutations))
	for i, m := range mutations {
		items[i] = toMutationOutput(m)
	}
	page, limit := normalizePagination(filter.Page, filter.Limit)
	return &usecase.MutationListOutput{Items: items, Total: total, Page: page, Limit: limit}, nil
}

func (u *adminUsecase) GetProviderBalances(ctx context.Context) ([]*entity.ProviderBalance, error) {
	return u.providerBalanceRepo.ListAll(ctx)
}

func (u *adminUsecase) UpdateProviderBalance(ctx context.Context, provider entity.Provider, balanceIDR int64) error {
	b := &entity.ProviderBalance{
		Provider:   provider,
		BalanceIDR: balanceIDR,
	}
	return u.providerBalanceRepo.Upsert(ctx, b)
}

// ── Admin management ──────────────────────────────────────────────────────────

func (u *adminUsecase) CreateAdmin(ctx context.Context, input usecase.CreateAdminInput) (*usecase.AdminOutput, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	a := &entity.Admin{
		Email:        input.Email,
		PasswordHash: string(hash),
		Role:         input.Role,
		IsActive:     true,
	}
	if err := u.adminRepo.Save(ctx, a); err != nil {
		return nil, fmt.Errorf("create admin: %w", err)
	}
	return toAdminOutput(a), nil
}

func (u *adminUsecase) ListAdmins(ctx context.Context, page, limit int) ([]*usecase.AdminOutput, int64, error) {
	admins, total, err := u.adminRepo.List(ctx, page, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("list admins: %w", err)
	}
	items := make([]*usecase.AdminOutput, len(admins))
	for i, a := range admins {
		items[i] = toAdminOutput(a)
	}
	return items, total, nil
}

func (u *adminUsecase) DeactivateAdmin(ctx context.Context, callerID, adminID string) error {
	if callerID == adminID {
		return apperror.BadRequest("cannot deactivate your own account")
	}
	a, err := u.adminRepo.FindByID(ctx, adminID)
	if err != nil {
		return err
	}
	a.IsActive = false
	return u.adminRepo.Update(ctx, a)
}

// ── Self ──────────────────────────────────────────────────────────────────────

func (u *adminUsecase) GetMe(ctx context.Context, adminID string) (*usecase.AdminOutput, error) {
	a, err := u.adminRepo.FindByID(ctx, adminID)
	if err != nil {
		return nil, err
	}
	return toAdminOutput(a), nil
}

func (u *adminUsecase) ChangePassword(ctx context.Context, adminID, oldPassword, newPassword string) error {
	a, err := u.adminRepo.FindByID(ctx, adminID)
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte(oldPassword)) != nil {
		return apperror.Unauthorized("incorrect current password")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}
	return u.adminRepo.UpdatePassword(ctx, adminID, string(hash))
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func toAdminOutput(a *entity.Admin) *usecase.AdminOutput {
	return &usecase.AdminOutput{
		ID:          a.ID,
		Email:       a.Email,
		Role:        a.Role,
		IsActive:    a.IsActive,
		LastLoginAt: a.LastLoginAt,
		CreatedAt:   a.CreatedAt,
	}
}

// normalizePagination returns safe page and limit values (mirrors postgres layer).
func normalizePagination(page, limit int) (int, int) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	return page, limit
}

func (u *adminUsecase) GetFeeDefault(ctx context.Context) (*entity.FeeDefault, error) {
	return u.feeRepo.GetDefault(ctx)
}

func (u *adminUsecase) UpdateFeeDefault(ctx context.Context, adminID string, fee entity.FeeConfig) error {
	f, err := u.feeRepo.GetDefault(ctx)
	if err != nil {
		return err
	}
	f.VA = fee.VA
	f.QRIS = fee.QRIS
	f.Disbursement = fee.Disbursement
	f.UpdatedBy = adminID
	return u.feeRepo.UpdateDefault(ctx, f)
}

func (u *adminUsecase) GetPlatformMargin(ctx context.Context) (*entity.PlatformMargin, error) {
	return u.feeRepo.GetMargin(ctx)
}

func (u *adminUsecase) UpdatePlatformMargin(ctx context.Context, adminID string, enabled bool, margin entity.FeeConfig) error {
	m, err := u.feeRepo.GetMargin(ctx)
	if err != nil {
		return err
	}
	m.Enabled = enabled
	m.VA = margin.VA
	m.QRIS = margin.QRIS
	m.Disbursement = margin.Disbursement
	m.UpdatedBy = adminID
	return u.feeRepo.UpdateMargin(ctx, m)
}
