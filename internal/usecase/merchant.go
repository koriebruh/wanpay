package usecase

import (
	"context"
	"time"

	"wanpey/core/internal/domain/entity"
)

type CreateMerchantInput struct {
	Name         string           `json:"name"          validate:"required,max=100"`
	Email        string           `json:"email"         validate:"required,email,max=255"`
	Phone        string           `json:"phone"         validate:"max=20"`
	WebhookURL   string           `json:"webhook_url"   validate:"omitempty,url,max=500"`
	IsProduction bool             `json:"is_production"`
	FeeConfig    entity.FeeConfig `json:"fee_config"`
}

type UpdateMerchantInput struct {
	MerchantID string `json:"-"`
	Name       string `json:"name"        validate:"max=100"`
	Email      string `json:"email"       validate:"omitempty,email,max=255"`
	Phone      string `json:"phone"       validate:"max=20"`
	WebhookURL string `json:"webhook_url" validate:"omitempty,url,max=500"`
}

type AddBankAccountInput struct {
	MerchantID    string          `json:"-"`
	BankCode      entity.BankCode `json:"bank_code"       validate:"required,oneof=BCA BNI BRI BSI MANDIRI PERMATA CIMB"`
	AccountNumber string          `json:"account_number"  validate:"required,min=5,max=20"`
	AccountName   string          `json:"account_name"    validate:"required,max=100"`
	SetAsPrimary  bool            `json:"set_as_primary"`
}

type CreateMerchantOutput struct {
	ID            string                `json:"id"`
	Name          string                `json:"name"`
	Status        entity.MerchantStatus `json:"status"`
	APIKey        string                `json:"api_key"`
	WebhookSecret string                `json:"webhook_secret"`
	CreatedAt     time.Time             `json:"created_at"`
}

type MerchantOutput struct {
	ID         string                `json:"id"`
	Name       string                `json:"name"`
	Email      string                `json:"email"`
	Phone      string                `json:"phone"`
	Status     entity.MerchantStatus `json:"status"`
	FeeConfig  entity.FeeConfig      `json:"fee_config"`
	WebhookURL string                `json:"webhook_url"`
	Balance    int64                 `json:"balance"`
	CreatedAt  time.Time             `json:"created_at"`
	UpdatedAt  time.Time             `json:"updated_at"`
}

type BankAccountOutput struct {
	ID            string          `json:"id"`
	BankCode      entity.BankCode `json:"bank_code"`
	AccountNumber string          `json:"account_number"`
	AccountName   string          `json:"account_name"`
	IsPrimary     bool            `json:"is_primary"`
	IsVerified    bool            `json:"is_verified"`
	CreatedAt     time.Time       `json:"created_at"`
}

type WebhookEventOutput struct {
	ID           string     `json:"id"`
	EventType    string     `json:"event_type"`
	TargetURL    string     `json:"target_url"`
	AttemptCount int32      `json:"attempt_count"`
	DeliveredAt  *time.Time `json:"delivered_at,omitempty"`
	FailedAt     *time.Time `json:"failed_at,omitempty"`
	NextRetryAt  time.Time  `json:"next_retry_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

type WebhookEventListOutput struct {
	Items []*WebhookEventOutput `json:"items"`
	Total int64                 `json:"total"`
	Page  int                   `json:"page"`
	Limit int                   `json:"limit"`
}

type MerchantUsecase interface {
	Create(ctx context.Context, input CreateMerchantInput) (*CreateMerchantOutput, error)
	GetMerchant(ctx context.Context, id string) (*MerchantOutput, error)
	Update(ctx context.Context, input UpdateMerchantInput) (*MerchantOutput, error)
	Suspend(ctx context.Context, merchantID string) error
	Activate(ctx context.Context, merchantID string) error
	RegenerateAPIKey(ctx context.Context, merchantID string) (rawKey string, err error)
	AddBankAccount(ctx context.Context, input AddBankAccountInput) (*BankAccountOutput, error)
	ListBankAccounts(ctx context.Context, merchantID string) ([]*BankAccountOutput, error)
	RemoveBankAccount(ctx context.Context, merchantID, accountID string) error
	SetPrimaryBankAccount(ctx context.Context, merchantID, accountID string) error
	ListWebhookEvents(ctx context.Context, merchantID string, page, limit int) (*WebhookEventListOutput, error)
}
