-- ============================================================
-- merchants
-- ============================================================
-- api_key and webhook_secret store SHA256 hashes — raw values shown once at creation only.
-- deleted_at is used for soft delete; records are never hard-deleted for financial compliance.
CREATE TABLE merchants (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT         NOT NULL,
    email           TEXT         NOT NULL,
    phone           TEXT         NOT NULL DEFAULT '',
    status          TEXT         NOT NULL DEFAULT 'pending',
    api_key         TEXT         NOT NULL,
    webhook_url     TEXT         NOT NULL DEFAULT '',
    webhook_secret  TEXT         NOT NULL DEFAULT '',
    fee_config      JSONB        NOT NULL DEFAULT '{}',
    deleted_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_merchant_status CHECK (status IN ('pending', 'active', 'suspended', 'inactive'))
);

CREATE UNIQUE INDEX uq_merchant_email   ON merchants (email)   WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX uq_merchant_api_key ON merchants (api_key) WHERE deleted_at IS NULL;

-- ============================================================
-- merchant_bank_accounts
-- ============================================================
-- At most MaxBankAccounts (3) per merchant — enforced at usecase layer.
-- Exactly one primary account per merchant — enforced via partial unique index.
-- is_verified must be true before disbursement to this account is allowed.
CREATE TABLE merchant_bank_accounts (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id    UUID        NOT NULL REFERENCES merchants(id),
    bank_code      TEXT        NOT NULL,
    account_number TEXT        NOT NULL,
    account_name   TEXT        NOT NULL,
    is_primary     BOOLEAN     NOT NULL DEFAULT false,
    is_verified    BOOLEAN     NOT NULL DEFAULT false,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_bank_code              CHECK (bank_code IN ('BCA', 'BNI', 'BRI', 'BSI', 'MANDIRI', 'PERMATA', 'CIMB')),
    CONSTRAINT uq_merchant_bank_account   UNIQUE (merchant_id, bank_code, account_number)
);

CREATE UNIQUE INDEX uq_merchant_primary_account ON merchant_bank_accounts (merchant_id)
    WHERE is_primary = true;

CREATE INDEX idx_bank_accounts_merchant ON merchant_bank_accounts (merchant_id);

-- ============================================================
-- payments
-- ============================================================
-- amount is the gross amount the customer pays; fee_amount is populated on webhook receipt.
-- external_id is the provider's reference — unique per provider for webhook correlation.
CREATE TABLE payments (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id     UUID         NOT NULL REFERENCES merchants(id),
    external_id     TEXT         NOT NULL,
    method          TEXT         NOT NULL,
    provider        TEXT         NOT NULL,
    status          TEXT         NOT NULL DEFAULT 'pending',
    amount          BIGINT       NOT NULL,
    fee_amount      BIGINT       NOT NULL DEFAULT 0,
    currency        TEXT         NOT NULL DEFAULT 'IDR',
    description     TEXT         NOT NULL DEFAULT '',
    customer_name   TEXT         NOT NULL DEFAULT '',
    customer_email  TEXT         NOT NULL DEFAULT '',
    customer_phone  TEXT         NOT NULL DEFAULT '',
    va_number       TEXT         NOT NULL DEFAULT '',
    bank_code       TEXT         NOT NULL DEFAULT '',
    qr_string       TEXT         NOT NULL DEFAULT '',
    qr_image_url    TEXT         NOT NULL DEFAULT '',
    expiry_at       TIMESTAMPTZ  NOT NULL,
    paid_at         TIMESTAMPTZ,
    failed_at       TIMESTAMPTZ,
    cancelled_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    metadata        JSONB        NOT NULL DEFAULT '{}',

    CONSTRAINT chk_payment_amount   CHECK (amount > 0),
    CONSTRAINT chk_payment_fee      CHECK (fee_amount >= 0 AND fee_amount <= amount),
    CONSTRAINT chk_payment_status   CHECK (status IN ('pending', 'paid', 'expired', 'failed', 'cancelled')),
    CONSTRAINT chk_payment_method   CHECK (method IN ('va', 'qris', 'cc')),
    CONSTRAINT chk_payment_provider CHECK (provider IN ('midtrans', 'xendit', 'doku')),
    CONSTRAINT chk_payment_currency CHECK (currency IN ('IDR')),
    CONSTRAINT chk_va_fields        CHECK (method != 'va'   OR (va_number != '' AND bank_code != '')),
    CONSTRAINT chk_qris_fields      CHECK (method != 'qris' OR qr_string != '')
);

CREATE UNIQUE INDEX uq_payment_external_id       ON payments (provider, external_id);
CREATE INDEX        idx_payments_merchant_created ON payments (merchant_id, created_at DESC);
CREATE INDEX        idx_payments_status_expiry    ON payments (status, expiry_at) WHERE status = 'pending';

-- ============================================================
-- disbursements
-- ============================================================
-- amount is gross sent to bank; fee_amount is deducted from merchant balance.
-- external_id starts empty and is set after provider confirms.
CREATE TABLE disbursements (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id     UUID         NOT NULL REFERENCES merchants(id),
    external_id     TEXT         NOT NULL DEFAULT '',
    provider        TEXT         NOT NULL,
    status          TEXT         NOT NULL DEFAULT 'pending',
    bank_code       TEXT         NOT NULL,
    account_number  TEXT         NOT NULL,
    account_name    TEXT         NOT NULL,
    amount          BIGINT       NOT NULL,
    fee_amount      BIGINT       NOT NULL DEFAULT 0,
    currency        TEXT         NOT NULL DEFAULT 'IDR',
    description     TEXT         NOT NULL DEFAULT '',
    failure_reason  TEXT         NOT NULL DEFAULT '',
    completed_at    TIMESTAMPTZ,
    failed_at       TIMESTAMPTZ,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_disbursement_amount    CHECK (amount > 0),
    CONSTRAINT chk_disbursement_fee       CHECK (fee_amount >= 0 AND fee_amount <= amount),
    CONSTRAINT chk_disbursement_status    CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
    CONSTRAINT chk_disbursement_provider  CHECK (provider IN ('midtrans', 'xendit', 'doku')),
    CONSTRAINT chk_disbursement_currency  CHECK (currency IN ('IDR')),
    CONSTRAINT chk_disbursement_bank_code CHECK (bank_code IN ('BCA', 'BNI', 'BRI', 'BSI', 'MANDIRI', 'PERMATA', 'CIMB'))
);

-- Only enforce uniqueness when external_id has been set by the provider.
CREATE UNIQUE INDEX uq_disbursement_external_id       ON disbursements (provider, external_id) WHERE external_id != '';
CREATE INDEX        idx_disbursements_merchant_created ON disbursements (merchant_id, created_at DESC);
CREATE INDEX        idx_disbursements_inflight         ON disbursements (status, updated_at) WHERE status IN ('pending', 'processing');

-- ============================================================
-- mutations (immutable ledger)
-- ============================================================
-- NEVER updated or deleted — single source of truth for merchant balance.
-- Cash-in: inserted when Payment → 'paid'.
-- Cash-out: inserted when Disbursement → 'completed'.
-- Net balance delta: cash_in → +(amount - fee_amount), cash_out → -amount.
CREATE TABLE mutations (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    reference_id    UUID         NOT NULL,
    reference_type  TEXT         NOT NULL,
    merchant_id     UUID         NOT NULL REFERENCES merchants(id),
    type            TEXT         NOT NULL,
    amount          BIGINT       NOT NULL,
    fee_amount      BIGINT       NOT NULL DEFAULT 0,
    currency        TEXT         NOT NULL DEFAULT 'IDR',
    description     TEXT         NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_mutation_amount    CHECK (amount > 0),
    CONSTRAINT chk_mutation_fee       CHECK (fee_amount >= 0 AND fee_amount <= amount),
    CONSTRAINT chk_mutation_type      CHECK (type IN ('cash_in', 'cash_out')),
    CONSTRAINT chk_mutation_ref_type  CHECK (reference_type IN ('payment', 'disbursement')),
    CONSTRAINT chk_mutation_currency  CHECK (currency IN ('IDR')),
    -- cash_out fee must be 0 — disbursement fees are charged at initiation
    CONSTRAINT chk_cash_out_fee       CHECK (type != 'cash_out' OR fee_amount = 0),
    -- one ledger entry per source event
    CONSTRAINT uq_mutation_reference  UNIQUE (reference_id, reference_type)
);

CREATE INDEX idx_mutations_balance          ON mutations (merchant_id, type, amount, fee_amount);
CREATE INDEX idx_mutations_merchant_created ON mutations (merchant_id, created_at DESC);
CREATE INDEX idx_mutations_reference        ON mutations (reference_id, reference_type);

-- ============================================================
-- payment_audits (append-only audit trail)
-- ============================================================
-- The app DB role should have INSERT + SELECT only on this table — no UPDATE, no DELETE.
-- old_status is NULL for PAYMENT_CREATED (no prior state).
-- actor format: 'system' | 'webhook:{provider}' | 'merchant:{id}'.
CREATE TABLE payment_audits (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_id  UUID         NOT NULL REFERENCES payments(id),
    event_type  TEXT         NOT NULL,
    old_status  TEXT,
    new_status  TEXT         NOT NULL,
    actor       TEXT         NOT NULL,
    metadata    JSONB        NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_audit_event_type CHECK (event_type IN (
        'PAYMENT_CREATED',
        'STATUS_CHANGED',
        'WEBHOOK_RECEIVED',
        'PAYMENT_CANCELLED',
        'PAYMENT_EXPIRED'
    )),
    CONSTRAINT chk_audit_new_status CHECK (new_status IN ('pending', 'paid', 'expired', 'failed', 'cancelled')),
    CONSTRAINT chk_audit_old_status CHECK (old_status IS NULL OR old_status IN ('pending', 'paid', 'expired', 'failed', 'cancelled'))
);

CREATE INDEX idx_payment_audits_payment_id ON payment_audits (payment_id, created_at ASC);
