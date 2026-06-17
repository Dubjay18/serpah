-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE users (
    id            UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    email         TEXT        UNIQUE NOT NULL,
    first_name    TEXT        NOT NULL DEFAULT '',
    last_name     TEXT        NOT NULL DEFAULT '',
    password_hash TEXT,
    is_active     BOOLEAN     NOT NULL DEFAULT true,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE refresh_tokens (
    id          UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT        UNIQUE NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tokens_user    ON refresh_tokens(user_id);
CREATE INDEX idx_tokens_hash    ON refresh_tokens(token_hash);
CREATE INDEX idx_tokens_expiry  ON refresh_tokens(expires_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS refresh_tokens CASCADE;
DROP TABLE IF EXISTS users CASCADE;
-- +goose StatementEnd
