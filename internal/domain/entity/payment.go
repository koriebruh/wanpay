package entity

import "time"

type PaymentStatus string

const (
	PaymentStatusPending   PaymentStatus = "pending"
	PaymentStatusPaid      PaymentStatus = "paid"
	PaymentStatusExpired   PaymentStatus = "expired"
	PaymentStatusFailed    PaymentStatus = "failed"
	PaymentStatusCancelled PaymentStatus = "cancelled"
)

type PaymentMethod string

const (
	PaymentMethodVA   PaymentMethod = "va"
	PaymentMethodQRIS PaymentMethod = "qris"
	PaymentMethodCC   PaymentMethod = "cc" // coming soon
)

type Provider string

const (
	ProviderMidtrans Provider = "midtrans"
	ProviderXendit   Provider = "xendit"
	ProviderDoku     Provider = "doku"
	ProviderIPaymu   Provider = "ipaymu"
)

type BankCode string

const (
	BankBCA     BankCode = "BCA"
	BankBNI     BankCode = "BNI"
	BankBRI     BankCode = "BRI"
	BankBSI     BankCode = "BSI"
	BankMandiri BankCode = "MANDIRI"
	BankPermata BankCode = "PERMATA"
	BankCIMB    BankCode = "CIMB"
)

type Currency string

const (
	CurrencyIDR Currency = "IDR"
)

// Payment represents a cash-in transaction initiated by a customer.
// Status transitions: pending → paid | expired | failed | cancelled.
// Once in a final state (IsFinal), the record must not be mutated — append payment_audits instead.
type Payment struct {
	ID            string
	MerchantID    string
	ExternalID    string // provider's reference ID, used for webhook correlation
	Method        PaymentMethod
	Provider      Provider
	Status        PaymentStatus
	Amount        int64 // gross amount paid by customer, smallest unit (IDR = 1 IDR)
	FeeAmount     int64 // fee charged to merchant; populated on webhook receipt (0 while pending)
	Currency      Currency
	Description   string
	CustomerName  string
	CustomerEmail string
	CustomerPhone string

	// VA-specific fields — empty for QRIS
	VANumber string
	BankCode BankCode

	// QRIS-specific fields — empty for VA
	QRString   string // raw QR string, rendered client-side
	QRImageURL string // hosted QR image URL, may be empty depending on provider

	ExpiryAt    time.Time
	PaidAt      *time.Time
	FailedAt    *time.Time
	CancelledAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Metadata    map[string]any
}

// IsFinal returns true when the payment cannot transition to any further state.
func (p *Payment) IsFinal() bool {
	switch p.Status {
	case PaymentStatusPaid, PaymentStatusExpired, PaymentStatusFailed, PaymentStatusCancelled:
		return true
	case PaymentStatusPending:
		return false
	}
	return false
}

// CanCancel returns true if the merchant is allowed to cancel this payment.
func (p *Payment) CanCancel() bool {
	return p.Status == PaymentStatusPending
}
