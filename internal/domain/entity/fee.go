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

type HolidayType string

const (
	HolidayTypeNational HolidayType = "national" // Lebaran, Natal, etc.
	HolidayTypeCustom   HolidayType = "custom"   // admin-defined special days
)

// FeeHoliday defines a surcharge added on top of the normal fee for a specific date.
// Surcharge is additive, not a replacement.
type FeeHoliday struct {
	ID        string
	Name      string      // e.g. "Idul Fitri 1446H"
	Date      time.Time   // DATE only (no time component)
	Type      HolidayType
	Surcharge MethodFee   // additional fee for ALL methods on this date
	IsActive  bool
	CreatedBy string // admin_id
	UpdatedBy string // admin_id
	CreatedAt time.Time
	UpdatedAt time.Time
}

// FeeAuditLog records every fee change — who changed what, when, and why.
// Records are append-only; never updated or deleted.
type FeeAuditLog struct {
	ID         string
	EntityType string         // "global_default" | "merchant_fee" | "platform_margin" | "holiday_surcharge"
	EntityID   string         // merchant_id, holiday_id, or "singleton"
	AdminID    string
	AdminEmail string         // denormalized for readability without a join
	OldValue   map[string]any // nil for first-time creation
	NewValue   map[string]any
	Reason     string
	CreatedAt  time.Time
}

const (
	FeeAuditEntityGlobalDefault   = "global_default"
	FeeAuditEntityMerchantFee     = "merchant_fee"
	FeeAuditEntityPlatformMargin  = "platform_margin"
	FeeAuditEntityHolidaySurcharge = "holiday_surcharge"
)
