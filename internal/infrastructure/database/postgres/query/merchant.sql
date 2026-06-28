-- name: InsertMerchant :one
INSERT INTO merchants (id, name, email, phone, status, api_key, webhook_url, webhook_secret, webhook_signing_key, fee_config)
VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetMerchantByID :one
SELECT * FROM merchants
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetMerchantByAPIKey :one
SELECT * FROM merchants
WHERE api_key = $1 AND deleted_at IS NULL;

-- name: GetMerchantByEmail :one
SELECT * FROM merchants
WHERE email = $1 AND deleted_at IS NULL;

-- name: UpdateMerchant :one
UPDATE merchants
SET name                = $2,
    email               = $3,
    phone               = $4,
    status              = $5,
    api_key             = $6,
    webhook_url         = $7,
    webhook_secret      = $8,
    webhook_signing_key = $9,
    fee_config          = $10,
    daily_cashout_limit = $11,
    updated_at          = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteMerchant :exec
UPDATE merchants
SET deleted_at = NOW(),
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: InsertBankAccount :one
INSERT INTO merchant_bank_accounts (id, merchant_id, bank_code, account_number, account_name, is_primary, is_verified)
VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListBankAccountsByMerchant :many
SELECT * FROM merchant_bank_accounts
WHERE merchant_id = $1
ORDER BY is_primary DESC, created_at ASC;

-- name: GetBankAccountByID :one
SELECT * FROM merchant_bank_accounts
WHERE id = $1;

-- name: GetPrimaryBankAccount :one
SELECT * FROM merchant_bank_accounts
WHERE merchant_id = $1 AND is_primary = true;

-- name: UpdateBankAccount :one
UPDATE merchant_bank_accounts
SET bank_code      = $2,
    account_number = $3,
    account_name   = $4,
    is_primary     = $5,
    is_verified    = $6,
    updated_at     = NOW()
WHERE id = $1
RETURNING *;

-- name: UnsetPrimaryBankAccounts :exec
UPDATE merchant_bank_accounts
SET is_primary = false,
    updated_at = NOW()
WHERE merchant_id = $1 AND is_primary = true;

-- name: DeleteBankAccount :exec
DELETE FROM merchant_bank_accounts WHERE id = $1;

-- name: CountBankAccounts :one
SELECT COUNT(*) FROM merchant_bank_accounts
WHERE merchant_id = $1;
