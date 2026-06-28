ALTER TABLE payments
    DROP CONSTRAINT chk_payment_status,
    ADD CONSTRAINT chk_payment_status
        CHECK (status IN ('pending', 'cancelling', 'paid', 'expired', 'failed', 'cancelled'));

ALTER TABLE payment_audits
    DROP CONSTRAINT chk_audit_new_status,
    ADD CONSTRAINT chk_audit_new_status
        CHECK (new_status IN ('pending', 'cancelling', 'paid', 'expired', 'failed', 'cancelled'));

ALTER TABLE payment_audits
    DROP CONSTRAINT chk_audit_old_status,
    ADD CONSTRAINT chk_audit_old_status
        CHECK (old_status IS NULL OR old_status IN ('pending', 'cancelling', 'paid', 'expired', 'failed', 'cancelled'));
