-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TYPE account_type   AS ENUM ('CHECKING', 'SAVINGS', 'FLOAT');
CREATE TYPE account_status AS ENUM ('ACTIVE', 'SUSPENDED', 'CLOSED');

CREATE SEQUENCE account_number_seq START 1000000000 INCREMENT 1;

CREATE TABLE accounts (
    id             UUID           PRIMARY KEY DEFAULT uuid_generate_v4(),
    owner_id       UUID           NOT NULL,
    account_number TEXT           UNIQUE NOT NULL DEFAULT lpad(nextval('account_number_seq')::text, 10, '0'),
    account_type   account_type   NOT NULL,
    currency       CHAR(3)        NOT NULL DEFAULT 'NGN',
    status         account_status NOT NULL DEFAULT 'ACTIVE',
    created_at     TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    closed_at      TIMESTAMPTZ
);

CREATE INDEX idx_accounts_owner ON accounts(owner_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS accounts CASCADE;
DROP SEQUENCE IF EXISTS account_number_seq;
DROP TYPE IF EXISTS account_status CASCADE;
DROP TYPE IF EXISTS account_type CASCADE;
-- +goose StatementEnd
