-- +goose Up
-- +goose StatementBegin

-- Enforce that payments are initiated using only platform-supported ISO-4217 currency codes.
ALTER TABLE payments
    ADD CONSTRAINT chk_payments_currency
    CHECK (currency IN ('NGN', 'USD', 'GBP', 'EUR'));

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE payments DROP CONSTRAINT IF EXISTS chk_payments_currency;
-- +goose StatementEnd
