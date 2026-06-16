CREATE TABLE outbox (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type      VARCHAR(50)  NOT NULL,
    payload         JSONB        NOT NULL,
    target_url      TEXT         NOT NULL,
    attempt_count   INT          NOT NULL DEFAULT 0,
    max_attempts    INT          NOT NULL DEFAULT 5,
    next_retry_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    delivered_at    TIMESTAMPTZ,
    failed_at       TIMESTAMPTZ,
    last_error      TEXT,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_attempt_count CHECK (attempt_count >= 0 AND attempt_count <= max_attempts)
);

CREATE INDEX idx_outbox_pending ON outbox (next_retry_at)
    WHERE delivered_at IS NULL AND attempt_count < max_attempts;
