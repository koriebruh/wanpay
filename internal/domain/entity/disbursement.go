package entity

import "time"

type DisbursementStatus string

const (
	DisbursementStatusPending    DisbursementStatus = "pending"
	DisbursementStatusProcessing DisbursementStatus = "processing"
	DisbursementStatusCompleted  DisbursementStatus = "completed"
	DisbursementStatusFailed     DisbursementStatus = "failed"
)

// Disbursement represents a cash-out transaction — money sent from the platform to a bank account.
// Unlike Payment (which is customer-initiated), disbursement is merchant/system-initiated.
type Disbursement struct {
	ID            string
	MerchantID    string
	ExternalID    string // provider's reference ID; empty until provider confirms
	Provider      Provider
	Status        DisbursementStatus
	BankCode      BankCode
	AccountNumber string
	AccountName   string
	Amount        int64 // gross amount disbursed in IDR
	FeeAmount     int64 // fee charged to merchant for this disbursement; deducted from balance
	Currency      Currency
	Description   string
	FailureReason string
	CompletedAt   *time.Time
	FailedAt      *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// IsFinal returns true when the disbursement cannot transition to any further state.
func (d *Disbursement) IsFinal() bool {
	return d.Status == DisbursementStatusCompleted || d.Status == DisbursementStatusFailed
}
