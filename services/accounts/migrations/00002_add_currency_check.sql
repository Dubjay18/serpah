-- +goose Up
-- +goose StatementBegin

-- Enforce that only platform-supported ISO-4217 currency codes are stored.
-- Update this list whenever shared/money/money.go adds a new currency.
ALTER TABLE accounts
    ADD CONSTRAINT chk_accounts_currency
    CHECK (currency IN ('NGN', 'USD', 'GBP', 'EUR'));

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE accounts DROP CONSTRAINT IF EXISTS chk_accounts_currency;
-- +goose StatementEnd
