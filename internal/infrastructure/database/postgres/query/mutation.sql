-- name: InsertMutation :one
INSERT INTO mutations (
    id, reference_id, reference_type, merchant_id,
    type, amount, fee_amount, currency, description
) VALUES (
    gen_random_uuid(), $1, $2, $3,
    $4, $5, $6, $7, $8
) RETURNING *;

-- name: GetMutationByID :one
SELECT * FROM mutations
WHERE id = $1;

-- name: GetMutationByReferenceID :one
SELECT * FROM mutations
WHERE reference_id = $1 AND reference_type = $2;

-- name: ListMutationsByMerchant :many
SELECT * FROM mutations
WHERE merchant_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountMutationsByMerchant :one
SELECT COUNT(*) FROM mutations
WHERE merchant_id = $1;

-- name: GetMerchantBalance :one
SELECT COALESCE(
    SUM(CASE
        WHEN type = 'cash_in'  THEN amount - fee_amount
        WHEN type = 'cash_out' THEN -amount
        ELSE 0
    END), 0
)::BIGINT AS balance
FROM mutations
WHERE merchant_id = $1;
