-- Dev seed: sample merchants for local development / Postman testing.
-- api_key stored as SHA256 hash. Raw keys:
--   merchant-alpha  → wpay_test_devkey0000000000000001 (hash below)
--   merchant-beta   → wpay_test_devkey0000000000000002
-- webhook_signing_key → raw HMAC key used to sign outbound webhooks.

-- ── Merchant Alpha (active, has bank account) ─────────────────────────────────
INSERT INTO merchants (
    id, name, email, phone, status,
    api_key, webhook_url, webhook_secret, webhook_signing_key,
    fee_config, daily_cashout_limit, is_production
)
VALUES (
    'b0000000-0000-0000-0000-000000000001',
    'Toko Alpha Dev',
    'alpha@merchant.dev',
    '081111111111',
    'active',
    -- SHA256("wpay_test_devkey0000000000000001")
    'b3a2c6e1d4f7890123456789abcdef0123456789abcdef0123456789abcdef01',
    'http://localhost:9090/webhook',
    -- SHA256("devsecret-alpha")
    'c3a2b1e0f9d8c7b6a5948372615049382716058493726150493827160584930',
    'devsecret-alpha-signing-key-32ch',
    '{"va":{"type":"flat","amount":4000},"qris":{"type":"percentage","percentage":0.7},"disbursement":{"type":"flat","amount":6500}}',
    10000000,
    false
)
ON CONFLICT DO NOTHING;

-- ── Merchant Beta (active, no custom fee — uses global default) ───────────────
INSERT INTO merchants (
    id, name, email, phone, status,
    api_key, webhook_url, webhook_secret, webhook_signing_key,
    fee_config, daily_cashout_limit, is_production
)
VALUES (
    'b0000000-0000-0000-0000-000000000002',
    'Warung Beta Dev',
    'beta@merchant.dev',
    '082222222222',
    'active',
    -- SHA256("wpay_test_devkey0000000000000002")
    'a1b2c3d4e5f60123456789abcdef0123456789abcdef0123456789abcdef0002',
    'http://localhost:9090/webhook',
    'c3a2b1e0f9d8c7b6a5948372615049382716058493726150493827160584931',
    'devsecret-beta-signing-key-32ch_',
    '{}',
    0,
    false
)
ON CONFLICT DO NOTHING;

-- ── Merchant Gamma (pending — not yet approved) ───────────────────────────────
INSERT INTO merchants (
    id, name, email, phone, status,
    api_key, webhook_url, webhook_secret, webhook_signing_key,
    fee_config, daily_cashout_limit, is_production
)
VALUES (
    'b0000000-0000-0000-0000-000000000003',
    'Startup Gamma Dev',
    'gamma@merchant.dev',
    '083333333333',
    'pending',
    'f1e2d3c4b5a60123456789abcdef0123456789abcdef0123456789abcdef0003',
    '',
    'c3a2b1e0f9d8c7b6a5948372615049382716058493726150493827160584932',
    'devsecret-gamma-signing-key-32ch',
    '{}',
    0,
    false
)
ON CONFLICT DO NOTHING;

-- ── Bank accounts for Merchant Alpha ─────────────────────────────────────────
INSERT INTO merchant_bank_accounts (id, merchant_id, bank_code, account_number, account_name, is_primary, is_verified)
VALUES
    ('c0000000-0000-0000-0000-000000000001', 'b0000000-0000-0000-0000-000000000001', 'BCA',  '1234567890', 'Toko Alpha Dev',  true,  true),
    ('c0000000-0000-0000-0000-000000000002', 'b0000000-0000-0000-0000-000000000001', 'BNI',  '9876543210', 'Toko Alpha Dev',  false, false)
ON CONFLICT DO NOTHING;

-- ── Bank accounts for Merchant Beta ──────────────────────────────────────────
INSERT INTO merchant_bank_accounts (id, merchant_id, bank_code, account_number, account_name, is_primary, is_verified)
VALUES
    ('c0000000-0000-0000-0000-000000000003', 'b0000000-0000-0000-0000-000000000002', 'MANDIRI', '1122334455', 'Warung Beta Dev', true, true)
ON CONFLICT DO NOTHING;
