package entity

import "time"

type MutationType string

const (
	MutationTypeCashIn  MutationType = "cash_in"
	MutationTypeCashOut MutationType = "cash_out"
)

// MutationReferenceType identifies whether ReferenceID points to a Payment or Disbursement.
type MutationReferenceType string

const (
	MutationRefPayment      MutationReferenceType = "payment"
	MutationRefDisbursement MutationReferenceType = "disbursement"
)

// Mutation is an immutable ledger record created for every completed financial event.
// Cash-in: created when a Payment reaches PaymentStatusPaid.
// Cash-out: created when a Disbursement reaches DisbursementStatusCompleted.
// Mutations are never updated or deleted — they are the single source of truth for merchant balance.
//
// Amount is the gross transaction amount (what the customer paid or what was disbursed).
// FeeAmount is the total fee charged to the merchant (platform margin + contracted fee).
// For cash_out, FeeAmount is always 0 — the disbursement fee is deducted at initiation.
// Effective balance delta: cash_in → +(Amount - FeeAmount), cash_out → -Amount.
type Mutation struct {
	ID            string
	ReferenceID   string                // PaymentID or DisbursementID
	ReferenceType MutationReferenceType // payment | disbursement — required for JOIN clarity
	MerchantID    string
	Type          MutationType
	Amount        int64 // gross transaction amount in IDR
	FeeAmount     int64 // total fee deducted from merchant (cash_in only; 0 for cash_out)
	Currency      Currency
	Description   string
	CreatedAt     time.Time
}

// NetAmount returns the effective amount that changes the merchant balance.
// cash_in: Amount - FeeAmount (net credited). cash_out: Amount (net debited).
func (m *Mutation) NetAmount() int64 {
	if m.Type == MutationTypeCashIn {
		return m.Amount - m.FeeAmount
	}
	return m.Amount
}
