-- name: InsertFeeHoliday :one
INSERT INTO fee_holidays (name, date, type, surcharge, is_active, created_by, updated_by)
VALUES ($1, $2, $3, $4, $5, $6, $6)
RETURNING *;

-- name: GetFeeHolidayByDate :one
SELECT * FROM fee_holidays WHERE date = $1 AND is_active = TRUE;

-- name: GetFeeHolidayByID :one
SELECT * FROM fee_holidays WHERE id = $1;

-- name: UpdateFeeHoliday :one
UPDATE fee_holidays
SET name       = $2,
    date       = $3,
    type       = $4,
    surcharge  = $5,
    is_active  = $6,
    updated_by = $7,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: ListFeeHolidays :many
SELECT * FROM fee_holidays
ORDER BY date DESC
LIMIT $1 OFFSET $2;

-- name: CountFeeHolidays :one
SELECT COUNT(*) FROM fee_holidays;

-- name: InsertFeeAuditLog :exec
INSERT INTO fee_audit_logs (entity_type, entity_id, admin_id, admin_email, old_value, new_value, reason)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListFeeAuditLogs :many
SELECT * FROM fee_audit_logs
WHERE entity_type = $1 AND entity_id = $2
ORDER BY created_at DESC
LIMIT $3 OFFSET $4;

-- name: GetFeeDefault :one
SELECT * FROM fee_defaults LIMIT 1;

-- name: UpdateFeeDefault :one
UPDATE fee_defaults
SET va           = $1,
    qris         = $2,
    disbursement = $3,
    updated_by   = $4,
    updated_at   = NOW()
RETURNING *;

-- name: GetPlatformMargin :one
SELECT * FROM platform_margin LIMIT 1;

-- name: UpdatePlatformMargin :one
UPDATE platform_margin
SET enabled      = $1,
    va           = $2,
    qris         = $3,
    disbursement = $4,
    updated_by   = $5,
    updated_at   = NOW()
RETURNING *;
