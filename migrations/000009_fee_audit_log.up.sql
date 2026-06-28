CREATE TABLE fee_audit_logs (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_type  TEXT        NOT NULL,
    entity_id    TEXT        NOT NULL,
    admin_id     UUID        NOT NULL REFERENCES admins(id),
    admin_email  TEXT        NOT NULL,
    old_value    JSONB,
    new_value    JSONB       NOT NULL,
    reason       TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_fee_audit_entity_type CHECK (entity_type IN (
        'global_default', 'merchant_fee', 'platform_margin', 'holiday_surcharge'
    ))
);

CREATE INDEX idx_fee_audit_logs_entity ON fee_audit_logs (entity_type, entity_id);
CREATE INDEX idx_fee_audit_logs_admin  ON fee_audit_logs (admin_id);
