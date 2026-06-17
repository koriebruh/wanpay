package entity

import "time"

type MerchantStatus string

const (
	MerchantStatusPending   MerchantStatus = "pending" // awaiting KYC/approval
	MerchantStatusActive    MerchantStatus = "active"
	MerchantStatusSuspended MerchantStatus = "suspended" // temporarily blocked
	MerchantStatusInactive  MerchantStatus = "inactive"  // closed account
)

type FeeType string

const (
	FeeTypeFlat       FeeType = "flat"       // fixed IDR amount per transaction
	FeeTypePercentage FeeType = "percentage" // % of transaction amount
)

// MaxBankAccounts is the maximum number of bank accounts a merchant may register.
const MaxBankAccounts = 3

// MethodFee defines the fee charged to the merchant for a single payment method.
// FeeBearer is always merchant — the fee is deducted from the settlement amount.
type MethodFee struct {
	Type       FeeType
	Amount     int64   // IDR, used when Type = flat
	Percentage float64 // e.g. 1.5 = 1.5%, used when Type = percentage
}

// FeeConfig holds per-method fee rules for a merchant.
// These are the fees Wanpey charges the merchant on top of provider costs.
// Platform margin (if enabled in config) is added on top of these values at settlement time.
type FeeConfig struct {
	VA           MethodFee
	QRIS         MethodFee
	Disbursement MethodFee
}

// MerchantBankAccount is a bank account registered by a merchant for disbursement.
// A merchant may have at most MaxBankAccounts accounts; exactly one must be primary.
// IsVerified must be true before the account can receive funds — enforced at usecase layer.
type MerchantBankAccount struct {
	ID            string
	MerchantID    string
	BankCode      BankCode
	AccountNumber string
	AccountName   string
	IsPrimary     bool // default account used when no account is specified in DisburseInput
	IsVerified    bool // must be true before disbursement to this account is allowed
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Merchant represents a business that processes payments through Wanpey.
//
// APIKey and WebhookSecret are stored as hashed values in the DB.
// Raw values are returned once at creation / regeneration and never stored plain.
// APIKey format: wpay_live_<32 random chars> (production) | wpay_test_<32 random chars> (sandbox).
// DeletedAt is set on soft delete — records are never hard-deleted for financial compliance.
type Merchant struct {
	ID            string
	Name          string
	Email         string
	Phone         string
	Status        MerchantStatus
	APIKey        string // SHA256 hash of the raw key
	WebhookURL    string // Wanpey POSTs payment events here
	WebhookSecret string // SHA256 hash; used to sign outbound webhook payloads via HMAC-SHA256
	FeeConfig     FeeConfig
	DeletedAt     *time.Time // nil = active record; soft delete only, never hard-delete
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (m *Merchant) IsActive() bool {
	return m.Status == MerchantStatusActive
}

func (m *Merchant) CanTransact() bool {
	return m.Status == MerchantStatusActive && m.WebhookURL != ""
}
