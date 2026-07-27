CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS notifications (
    id            UUID PRIMARY KEY,
    channel       VARCHAR(20)  NOT NULL CHECK (channel IN ('email', 'sms', 'push')),
    recipient     VARCHAR(255) NOT NULL,
    subject       VARCHAR(255),
    body          TEXT         NOT NULL,
    status        VARCHAR(20)  NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'sent', 'failed', 'retrying')),
    attempts      INT          NOT NULL DEFAULT 0,
    max_attempts  INT          NOT NULL DEFAULT 3,
    metadata      JSONB,
    error         TEXT,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    sent_at       TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_notifications_status ON notifications (status);
CREATE INDEX IF NOT EXISTS idx_notifications_channel ON notifications (channel);
CREATE INDEX IF NOT EXISTS idx_notifications_created_at ON notifications (created_at DESC);

CREATE TABLE IF NOT EXISTS templates (
    id          UUID PRIMARY KEY,
    name        VARCHAR(100) NOT NULL UNIQUE,
    channel     VARCHAR(20)  NOT NULL CHECK (channel IN ('email', 'sms', 'push')),
    subject     TEXT,
    body        TEXT         NOT NULL,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);
