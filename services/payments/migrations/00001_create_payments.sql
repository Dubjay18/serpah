-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TYPE payment_status AS ENUM (
    'INITIATED', 'PROCESSING', 'COMPLETED', 'FAILED', 'REVERSED'
);

CREATE TABLE payments (
    id                   UUID           PRIMARY KEY DEFAULT uuid_generate_v4(),
    idempotency_key      TEXT           UNIQUE NOT NULL,
    from_account_id      UUID           NOT NULL,
    to_account_id        UUID           NOT NULL,
    amount               BIGINT         NOT NULL CHECK (amount > 0),
    currency             CHAR(3)        NOT NULL DEFAULT 'NGN',
    status               payment_status NOT NULL DEFAULT 'INITIATED',
    ledger_transaction_id UUID,
    description          TEXT,
    created_at           TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ    NOT NULL DEFAULT NOW()
);

-- Transactional outbox for reliable event publishing
CREATE TABLE outbox_events (
    id           UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    event_type   TEXT        NOT NULL,
    payload      JSONB       NOT NULL,
    published_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_outbox_unpublished ON outbox_events(created_at) WHERE published_at IS NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS outbox_events CASCADE;
DROP TABLE IF EXISTS payments CASCADE;
DROP TYPE IF EXISTS payment_status CASCADE;
-- +goose StatementEnd
