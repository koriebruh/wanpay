-- Global fallback fee when a merchant has no custom fee_config.
CREATE TABLE fee_defaults (
    id         UUID      PRIMARY KEY DEFAULT gen_random_uuid(),
    va         JSONB     NOT NULL DEFAULT '{}',
    qris       JSONB     NOT NULL DEFAULT '{}',
    disbursement JSONB   NOT NULL DEFAULT '{}',
    updated_by TEXT      NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seed one row. All zeros = no global default (transactions fail with explicit error).
INSERT INTO fee_defaults (va, qris, disbursement)
VALUES (
    '{"type":"flat","amount":4500,"percentage":0}',
    '{"type":"percentage","amount":0,"percentage":0.7}',
    '{"type":"flat","amount":5000,"percentage":0}'
);

-- Platform-wide margin added on top of merchant fee. One row, always present.
CREATE TABLE platform_margin (
    id           UUID      PRIMARY KEY DEFAULT gen_random_uuid(),
    enabled      BOOLEAN   NOT NULL DEFAULT FALSE,
    va           JSONB     NOT NULL DEFAULT '{}',
    qris         JSONB     NOT NULL DEFAULT '{}',
    disbursement JSONB     NOT NULL DEFAULT '{}',
    updated_by   TEXT      NOT NULL DEFAULT '',
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seed one disabled row.
INSERT INTO platform_margin (enabled, va, qris, disbursement)
VALUES (
    FALSE,
    '{"type":"flat","amount":500,"percentage":0}',
    '{"type":"percentage","amount":0,"percentage":0.1}',
    '{"type":"flat","amount":1000,"percentage":0}'
);
