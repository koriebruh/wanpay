package usecase

import (
	"context"
	"time"

	"wanpey/core/internal/domain/entity"
)

type CreateVAInput struct {
	MerchantID    string          `json:"-"`
	Provider      entity.Provider `json:"provider"       validate:"required,oneof=midtrans xendit doku"`
	BankCode      entity.BankCode `json:"bank_code"      validate:"required,oneof=BCA BNI BRI BSI MANDIRI PERMATA CIMB"`
	Amount        int64           `json:"amount"         validate:"required,gt=0"`
	Currency      entity.Currency `json:"currency"       validate:"required,oneof=IDR"`
	CustomerName  string          `json:"customer_name"  validate:"required,max=100"`
	CustomerEmail string          `json:"customer_email" validate:"required,email,max=255"`
	CustomerPhone string          `json:"customer_phone" validate:"required,min=6,max=20"`
	Description   string          `json:"description"    validate:"max=255"`
	ExpiryAt      time.Time       `json:"expiry_at"      validate:"required"`
}

type CreateQRISInput struct {
	MerchantID    string          `json:"-"`
	Provider      entity.Provider `json:"provider"       validate:"required,oneof=midtrans xendit doku"`
	Amount        int64           `json:"amount"         validate:"required,gt=0"`
	Currency      entity.Currency `json:"currency"       validate:"required,oneof=IDR"`
	CustomerName  string          `json:"customer_name"  validate:"required,max=100"`
	CustomerEmail string          `json:"customer_email" validate:"required,email,max=255"`
	CustomerPhone string          `json:"customer_phone" validate:"required,min=6,max=20"`
	Description   string          `json:"description"    validate:"max=255"`
	ExpiryAt      time.Time       `json:"expiry_at"      validate:"required"`
}

type PaymentOutput struct {
	ID            string               `json:"id"`
	ExternalID    string               `json:"external_id"`
	Method        entity.PaymentMethod `json:"method"`
	Provider      entity.Provider      `json:"provider"`
	Status        entity.PaymentStatus `json:"status"`
	Amount        int64                `json:"amount"`
	FeeAmount     int64                `json:"fee_amount"`
	Currency      entity.Currency      `json:"currency"`
	CustomerName  string               `json:"customer_name"`
	CustomerEmail string               `json:"customer_email"`
	VANumber      string               `json:"va_number,omitempty"`
	BankCode      entity.BankCode      `json:"bank_code,omitempty"`
	QRString      string               `json:"qr_string,omitempty"`
	QRImageURL    string               `json:"qr_image_url,omitempty"`
	ExpiryAt      time.Time            `json:"expiry_at"`
	PaidAt        *time.Time           `json:"paid_at,omitempty"`
	CancelledAt   *time.Time           `json:"cancelled_at,omitempty"`
	CreatedAt     time.Time            `json:"created_at"`
}

type PaymentUsecase interface {
	CreateVA(ctx context.Context, input CreateVAInput) (*PaymentOutput, error)
	CreateQRIS(ctx context.Context, input CreateQRISInput) (*PaymentOutput, error)
	GetPayment(ctx context.Context, merchantID, paymentID string) (*PaymentOutput, error)
	CancelPayment(ctx context.Context, merchantID, paymentID string) error
	HandleWebhook(ctx context.Context, provider entity.Provider, headers map[string]string, body []byte) error
}
