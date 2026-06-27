ALTER TABLE payments
    ADD COLUMN IF NOT EXISTS provider_payment_id TEXT NOT NULL DEFAULT '';
