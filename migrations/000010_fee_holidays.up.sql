CREATE TABLE fee_holidays (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT        NOT NULL,
    date       DATE        NOT NULL,
    type       TEXT        NOT NULL DEFAULT 'custom',
    surcharge  JSONB       NOT NULL DEFAULT '{}',
    is_active  BOOLEAN     NOT NULL DEFAULT TRUE,
    created_by TEXT        NOT NULL DEFAULT '',
    updated_by TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_holiday_type CHECK (type IN ('national', 'custom')),
    CONSTRAINT uq_holiday_date UNIQUE (date)
);

CREATE INDEX idx_fee_holidays_date ON fee_holidays (date) WHERE is_active = TRUE;
