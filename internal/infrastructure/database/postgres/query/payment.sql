-- name: InsertPayment :one
INSERT INTO payments (
    id, merchant_id, external_id, method, provider, status,
    amount, fee_amount, currency, description,
    customer_name, customer_email, customer_phone,
    va_number, bank_code, qr_string, qr_image_url,
    expiry_at, metadata
) VALUES (
    gen_random_uuid(), $1, $2, $3, $4, $5,
    $6, $7, $8, $9,
    $10, $11, $12,
    $13, $14, $15, $16,
    $17, $18
) RETURNING *;

-- name: GetPaymentByID :one
SELECT * FROM payments
WHERE id = $1;

-- name: GetPaymentByExternalID :one
SELECT * FROM payments
WHERE provider = $1 AND external_id = $2;

-- name: UpdatePaymentStatus :one
UPDATE payments
SET status       = $2,
    fee_amount   = $3,
    paid_at      = $4,
    failed_at    = $5,
    cancelled_at = $6,
    updated_at   = NOW()
WHERE id = $1
RETURNING *;

-- name: ListPaymentsByMerchant :many
SELECT * FROM payments
WHERE merchant_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountPaymentsByMerchant :one
SELECT COUNT(*) FROM payments
WHERE merchant_id = $1;
