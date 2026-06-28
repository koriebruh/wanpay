package entity

import "time"

// FeeDefault is the platform-wide fallback fee used when a merchant has no custom FeeConfig.
// There is exactly one active row — admin updates it in-place, never inserts a new one.
type FeeDefault struct {
	ID           string
	VA           MethodFee
	QRIS         MethodFee
	Disbursement MethodFee
	UpdatedBy    string // admin_id
	UpdatedAt    time.Time
	CreatedAt    time.Time
}

// PlatformMargin is Wanpey's revenue layer added on top of every merchant fee.
// There is exactly one row — super_admin manages it via admin API.
// Stored in DB so it can be updated without a server restart.
type PlatformMargin struct {
	ID           string
	Enabled      bool
	VA           MethodFee
	QRIS         MethodFee
	Disbursement MethodFee
	UpdatedBy    string // admin_id
	UpdatedAt    time.Time
	CreatedAt    time.Time
}
