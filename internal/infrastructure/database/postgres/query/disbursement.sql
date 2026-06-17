-- name: InsertDisbursement :one
INSERT INTO disbursements (
    id, merchant_id, external_id, provider, status,
    bank_code, account_number, account_name,
    amount, fee_amount, currency, description
) VALUES (
    gen_random_uuid(), $1, $2, $3, $4,
    $5, $6, $7,
    $8, $9, $10, $11
) RETURNING *;

-- name: GetDisbursementByID :one
SELECT * FROM disbursements
WHERE id = $1;

-- name: GetDisbursementByExternalID :one
SELECT * FROM disbursements
WHERE provider = $1 AND external_id = $2;

-- name: UpdateDisbursementStatus :one
UPDATE disbursements
SET status         = $2,
    external_id    = $3,
    failure_reason = $4,
    completed_at   = $5,
    failed_at      = $6,
    updated_at     = NOW()
WHERE id = $1
RETURNING *;

-- name: ListDisbursementsByMerchant :many
SELECT * FROM disbursements
WHERE merchant_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountDisbursementsByMerchant :one
SELECT COUNT(*) FROM disbursements
WHERE merchant_id = $1;
