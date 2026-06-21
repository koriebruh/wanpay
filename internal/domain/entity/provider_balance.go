package entity

import "time"

// ProviderBalance tracks the Wanpey platform's balance held at each payment provider.
// In the PayFac model, Wanpey holds one account per provider — all merchant payments
// flow through these accounts. This table is used for audit and reconciliation.
type ProviderBalance struct {
	ID               string
	Provider         Provider
	BalanceIDR       int64      // last known balance; updated after settlement or manual reconciliation
	LastReconciledAt *time.Time // nil = never reconciled
	Note             string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
