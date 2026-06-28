ALTER TABLE merchants
    ADD COLUMN webhook_signing_key TEXT NOT NULL DEFAULT '';

ALTER TABLE outbox
    ADD COLUMN merchant_id UUID REFERENCES merchants(id);
