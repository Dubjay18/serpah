-- +goose Up
-- +goose StatementBegin

-- Allow OAuth-only accounts to have no password
ALTER TABLE users ALTER COLUMN password_hash DROP NOT NULL;

-- Links a Google (or future provider) account to an internal user
CREATE TABLE oauth_identities (
    id          UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider    TEXT        NOT NULL,  -- e.g. "google"
    provider_id TEXT        NOT NULL,  -- Google's stable `sub` claim
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (provider, provider_id)
);

CREATE INDEX idx_oauth_user ON oauth_identities(user_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS oauth_identities CASCADE;
ALTER TABLE users ALTER COLUMN password_hash SET NOT NULL;
-- +goose StatementEnd
