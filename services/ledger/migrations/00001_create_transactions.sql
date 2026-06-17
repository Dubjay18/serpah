-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TYPE entry_type AS ENUM ('DEBIT', 'CREDIT');
CREATE TYPE txn_status AS ENUM ('PENDING', 'POSTED', 'REVERSED', 'FAILED');

CREATE TABLE transactions (
    id               UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    idempotency_key  TEXT        UNIQUE NOT NULL,
    status           txn_status  NOT NULL DEFAULT 'PENDING',
    description      TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    posted_at        TIMESTAMPTZ
);

CREATE TABLE ledger_entries (
    id             UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    transaction_id UUID        NOT NULL REFERENCES transactions(id),
    account_id     UUID        NOT NULL,
    entry_type     entry_type  NOT NULL,
    -- Amount stored in KOBO (smallest currency unit). NEVER floats.
    amount         BIGINT      NOT NULL CHECK (amount > 0),
    currency       CHAR(3)     NOT NULL DEFAULT 'NGN',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_entries_account_created ON ledger_entries(account_id, created_at DESC);
CREATE INDEX idx_entries_transaction     ON ledger_entries(transaction_id);

-- Idempotency key store
CREATE TABLE idempotency_keys (
    key            TEXT        PRIMARY KEY,
    transaction_id UUID        NOT NULL REFERENCES transactions(id),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Balance view: always derived, never stored
CREATE VIEW account_balances AS
SELECT
    account_id,
    currency,
    SUM(CASE WHEN entry_type = 'CREDIT' THEN amount ELSE -amount END) AS balance_kobo
FROM ledger_entries
GROUP BY account_id, currency;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP VIEW IF EXISTS account_balances;
DROP TABLE IF EXISTS idempotency_keys;
DROP TABLE IF EXISTS ledger_entries;
DROP TABLE IF EXISTS transactions;
DROP TYPE IF EXISTS txn_status;
DROP TYPE IF EXISTS entry_type;
-- +goose StatementEnd
