ALTER TABLE outbox DROP COLUMN IF EXISTS merchant_id;
ALTER TABLE merchants DROP COLUMN IF EXISTS webhook_signing_key;
