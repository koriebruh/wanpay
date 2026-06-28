CREATE TABLE admins (
    id            TEXT        NOT NULL DEFAULT gen_random_uuid()::TEXT,
    email         TEXT        NOT NULL,
    password_hash TEXT        NOT NULL,
    role          TEXT        NOT NULL DEFAULT 'ops'
                              CHECK (role IN ('super_admin', 'ops', 'finance')),
    is_active     BOOLEAN     NOT NULL DEFAULT true,
    last_login_at TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT pk_admins PRIMARY KEY (id),
    CONSTRAINT uq_admins_email UNIQUE (email)
);
