-- Dev seed: sample payments and mutations for Merchant Alpha.
-- Useful for testing list, filter, and balance endpoints.

-- ── Payments ──────────────────────────────────────────────────────────────────

-- VA payment (paid)
INSERT INTO payments (
    id, merchant_id, external_id, method, provider, status,
    amount, fee_amount, currency, description,
    customer_name, customer_email, customer_phone,
    va_number, bank_code, qr_string, qr_image_url,
    expiry_at, paid_at, metadata
)
VALUES (
    'd0000000-0000-0000-0000-000000000001',
    'b0000000-0000-0000-0000-000000000001',
    'wpay-seed-va-001',
    'va', 'bca', 'paid',
    500000, 4000, 'IDR', 'Order #SEED-001',
    'Budi Santoso', 'budi@example.com', '081234567890',
    '8023123456789', 'BCA', '', '',
    NOW() + INTERVAL '24 hours',
    NOW() - INTERVAL '1 hour',
    '{}'
)
ON CONFLICT DO NOTHING;

-- QRIS payment (paid)
INSERT INTO payments (
    id, merchant_id, external_id, method, provider, status,
    amount, fee_amount, currency, description,
    customer_name, customer_email, customer_phone,
    va_number, bank_code, qr_string, qr_image_url,
    expiry_at, paid_at, metadata
)
VALUES (
    'd0000000-0000-0000-0000-000000000002',
    'b0000000-0000-0000-0000-000000000001',
    'wpay-seed-qris-001',
    'qris', 'xendit', 'paid',
    250000, 1750, 'IDR', 'Order #SEED-002',
    'Siti Rahayu', 'siti@example.com', '082345678901',
    '', '', '00020101021126...QRSTRING', 'https://qr.example.com/seed',
    NOW() + INTERVAL '15 minutes',
    NOW() - INTERVAL '30 minutes',
    '{}'
)
ON CONFLICT DO NOTHING;

-- VA payment (pending)
INSERT INTO payments (
    id, merchant_id, external_id, method, provider, status,
    amount, fee_amount, currency, description,
    customer_name, customer_email, customer_phone,
    va_number, bank_code, qr_string, qr_image_url,
    expiry_at, metadata
)
VALUES (
    'd0000000-0000-0000-0000-000000000003',
    'b0000000-0000-0000-0000-000000000001',
    'wpay-seed-va-002',
    'va', 'midtrans', 'pending',
    1000000, 0, 'IDR', 'Order #SEED-003 (pending)',
    'Ahmad Fauzi', 'ahmad@example.com', '083456789012',
    '7001987654321', 'MANDIRI', '', '',
    NOW() + INTERVAL '24 hours',
    '{}'
)
ON CONFLICT DO NOTHING;

-- VA payment (expired)
INSERT INTO payments (
    id, merchant_id, external_id, method, provider, status,
    amount, fee_amount, currency, description,
    customer_name, customer_email, customer_phone,
    va_number, bank_code, qr_string, qr_image_url,
    expiry_at, metadata
)
VALUES (
    'd0000000-0000-0000-0000-000000000004',
    'b0000000-0000-0000-0000-000000000001',
    'wpay-seed-va-003',
    'va', 'doku', 'expired',
    750000, 0, 'IDR', 'Order #SEED-004 (expired)',
    'Dewi Kusuma', 'dewi@example.com', '084567890123',
    '3331234567890', 'BNI', '', '',
    NOW() - INTERVAL '2 hours',
    '{}'
)
ON CONFLICT DO NOTHING;

-- ── Payment audits ─────────────────────────────────────────────────────────────

INSERT INTO payment_audits (payment_id, event_type, old_status, new_status, actor)
VALUES
    ('d0000000-0000-0000-0000-000000000001', 'PAYMENT_CREATED',  NULL,      'pending', 'merchant:b0000000-0000-0000-0000-000000000001'),
    ('d0000000-0000-0000-0000-000000000001', 'WEBHOOK_RECEIVED', 'pending', 'paid',    'webhook:bca'),
    ('d0000000-0000-0000-0000-000000000002', 'PAYMENT_CREATED',  NULL,      'pending', 'merchant:b0000000-0000-0000-0000-000000000001'),
    ('d0000000-0000-0000-0000-000000000002', 'WEBHOOK_RECEIVED', 'pending', 'paid',    'webhook:xendit'),
    ('d0000000-0000-0000-0000-000000000003', 'PAYMENT_CREATED',  NULL,      'pending', 'merchant:b0000000-0000-0000-0000-000000000001'),
    ('d0000000-0000-0000-0000-000000000004', 'PAYMENT_CREATED',  NULL,      'pending', 'merchant:b0000000-0000-0000-0000-000000000001'),
    ('d0000000-0000-0000-0000-000000000004', 'PAYMENT_EXPIRED',  'pending', 'expired', 'system')
ON CONFLICT DO NOTHING;

-- ── Mutations (cash-in from paid payments) ────────────────────────────────────

INSERT INTO mutations (id, reference_id, reference_type, merchant_id, type, amount, fee_amount, currency, description)
VALUES
    (
        'e0000000-0000-0000-0000-000000000001',
        'd0000000-0000-0000-0000-000000000001',
        'payment',
        'b0000000-0000-0000-0000-000000000001',
        'cash_in', 500000, 4000, 'IDR',
        'Payment Order #SEED-001'
    ),
    (
        'e0000000-0000-0000-0000-000000000002',
        'd0000000-0000-0000-0000-000000000002',
        'payment',
        'b0000000-0000-0000-0000-000000000001',
        'cash_in', 250000, 1750, 'IDR',
        'Payment Order #SEED-002'
    )
ON CONFLICT DO NOTHING;

-- ── Disbursement (completed) ──────────────────────────────────────────────────

INSERT INTO disbursements (
    id, merchant_id, external_id,
    provider, status, amount, fee_amount, currency,
    bank_code, account_number, account_name,
    description, completed_at
)
VALUES (
    'f0000000-0000-0000-0000-000000000001',
    'b0000000-0000-0000-0000-000000000001',
    'wpay-seed-disb-001',
    'xendit', 'completed',
    200000, 6500, 'IDR',
    'BCA', '1234567890', 'Toko Alpha Dev',
    'Withdrawal #SEED-001',
    NOW() - INTERVAL '30 minutes'
)
ON CONFLICT DO NOTHING;

-- cash-out mutation
INSERT INTO mutations (id, reference_id, reference_type, merchant_id, type, amount, fee_amount, currency, description)
VALUES (
    'e0000000-0000-0000-0000-000000000003',
    'f0000000-0000-0000-0000-000000000001',
    'disbursement',
    'b0000000-0000-0000-0000-000000000001',
    'cash_out', 200000, 0, 'IDR',
    'Withdrawal #SEED-001'
)
ON CONFLICT DO NOTHING;

-- ── Provider balances ─────────────────────────────────────────────────────────

INSERT INTO provider_balances (provider, balance_idr, note)
VALUES
    ('midtrans', 50000000,  'dev seed — not real'),
    ('xendit',   75000000,  'dev seed — not real'),
    ('doku',     25000000,  'dev seed — not real'),
    ('ipaymu',   10000000,  'dev seed — not real')
ON CONFLICT (provider) DO UPDATE
    SET balance_idr = EXCLUDED.balance_idr,
        note        = EXCLUDED.note,
        updated_at  = NOW();
