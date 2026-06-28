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
