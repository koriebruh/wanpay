package usecase

import (
	"context"
	"time"

	"wanpey/core/internal/domain/entity"
)

type AdminLoginInput struct {
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
}

type AdminTokenOutput struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type CreateAdminInput struct {
	RequesterID string           `json:"-"`
	Email       string           `json:"email"    validate:"required,email"`
	Password    string           `json:"password" validate:"required,min=8"`
	Role        entity.AdminRole `json:"role"     validate:"required,oneof=super_admin ops finance"`
}

type AdminOutput struct {
	ID          string           `json:"id"`
	Email       string           `json:"email"`
	Role        entity.AdminRole `json:"role"`
	IsActive    bool             `json:"is_active"`
	LastLoginAt *time.Time       `json:"last_login_at,omitempty"`
	CreatedAt   time.Time        `json:"created_at"`
}

type SetMerchantFeeInput struct {
	MerchantID string           `json:"-"`
	FeeConfig  entity.FeeConfig `json:"fee_config"`
	Reason     string           `json:"reason" validate:"required,min=10"`
}

type AdminListMerchantsFilter struct {
	Status string
	Search string
	Page   int
	Limit  int
}

type AdminPaymentFilter struct {
	MerchantID string
	Status     string
	Provider   string
	Start      string
	End        string
	Page       int
	Limit      int
}

type AdminDisbursementFilter struct {
	MerchantID string
	Status     string
	Provider   string
	Start      string
	End        string
	Page       int
	Limit      int
}

type AdminMutationFilter struct {
	MerchantID string
	Type       string
	Start      string
	End        string
	Page       int
	Limit      int
}

type MerchantListOutput struct {
	Items []*MerchantOutput `json:"items"`
	Total int64             `json:"total"`
	Page  int               `json:"page"`
	Limit int               `json:"limit"`
}


type AdminUsecase interface {
	Login(ctx context.Context, input AdminLoginInput) (*AdminTokenOutput, error)
	RefreshToken(ctx context.Context, refreshToken string) (*AdminTokenOutput, error)

	// Merchant management
	CreateMerchant(ctx context.Context, input CreateMerchantInput) (*CreateMerchantOutput, error)
	ListMerchants(ctx context.Context, filter AdminListMerchantsFilter) (*MerchantListOutput, error)
	GetMerchant(ctx context.Context, merchantID string) (*MerchantOutput, error)
	ApproveMerchant(ctx context.Context, merchantID string) error
	SuspendMerchant(ctx context.Context, merchantID string) error
	DeactivateMerchant(ctx context.Context, merchantID string) error
	DeleteMerchant(ctx context.Context, merchantID string) error
	UpdateMerchantFee(ctx context.Context, input SetMerchantFeeInput) error
	UpdateDailyCashoutLimit(ctx context.Context, merchantID string, limitIDR int64) error
	RegenerateMerchantAPIKey(ctx context.Context, merchantID string) (rawKey string, err error)

	// Bank account
	VerifyBankAccount(ctx context.Context, merchantID, accountID string) error
	ListMerchantBankAccounts(ctx context.Context, merchantID string) ([]*BankAccountOutput, error)

	// Visibility
	ListAllPayments(ctx context.Context, filter AdminPaymentFilter) (*PaymentListOutput, error)
	GetPayment(ctx context.Context, paymentID string) (*PaymentOutput, error)
	ListAllDisbursements(ctx context.Context, filter AdminDisbursementFilter) (*DisbursementListOutput, error)
	GetDisbursement(ctx context.Context, disbursementID string) (*DisbursementOutput, error)
	ListAllMutations(ctx context.Context, filter AdminMutationFilter) (*MutationListOutput, error)
	GetProviderBalances(ctx context.Context) ([]*entity.ProviderBalance, error)
	UpdateProviderBalance(ctx context.Context, provider entity.Provider, balanceIDR int64) error

	// Admin management (super_admin only)
	CreateAdmin(ctx context.Context, input CreateAdminInput) (*AdminOutput, error)
	ListAdmins(ctx context.Context, page, limit int) ([]*AdminOutput, int64, error)
	DeactivateAdmin(ctx context.Context, callerID, adminID string) error

	// Self
	GetMe(ctx context.Context, adminID string) (*AdminOutput, error)
	ChangePassword(ctx context.Context, adminID, oldPassword, newPassword string) error
}
