-- +goose Up
-- +goose StatementBegin

CREATE TABLE account_balances_cache (
    account_id  UUID        NOT NULL,
    currency    CHAR(3)     NOT NULL,
    balance     BIGINT      NOT NULL DEFAULT 0,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (account_id, currency),
    CONSTRAINT chk_balances_currency CHECK (currency IN ('NGN', 'USD', 'GBP', 'EUR'))
);

CREATE INDEX idx_balances_cache_account ON account_balances_cache(account_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS account_balances_cache;
-- +goose StatementEnd
