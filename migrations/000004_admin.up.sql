CREATE TABLE admins (
    id            TEXT        NOT NULL DEFAULT gen_random_uuid()::TEXT,
    username      TEXT        NOT NULL,
    password_hash TEXT        NOT NULL,
    role          TEXT        NOT NULL DEFAULT 'admin'
                              CHECK (role IN ('admin', 'super_admin')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT pk_admins PRIMARY KEY (id),
    CONSTRAINT uq_admins_username UNIQUE (username)
);
