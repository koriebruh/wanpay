-- name: UpsertProviderBalance :one
INSERT INTO provider_balances (id, provider, balance_idr, last_reconciled_at, note)
VALUES (gen_random_uuid(), $1, $2, $3, $4)
ON CONFLICT (provider) DO UPDATE
SET balance_idr         = EXCLUDED.balance_idr,
    last_reconciled_at  = EXCLUDED.last_reconciled_at,
    note                = EXCLUDED.note,
    updated_at          = NOW()
RETURNING *;

-- name: GetProviderBalance :one
SELECT * FROM provider_balances
WHERE provider = $1;

-- name: ListProviderBalances :many
SELECT * FROM provider_balances
ORDER BY provider ASC;
