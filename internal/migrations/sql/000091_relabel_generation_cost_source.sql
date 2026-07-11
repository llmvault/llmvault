-- +goose Up
-- The proxy computes positive generation costs from registry prices. Older
-- billing batches mislabeled those precomputed values as provider-reported.
UPDATE generations
SET billing_cost_source = 'registry_estimated'
WHERE billing_cost_source = 'provider_reported';

-- +goose Down
-- The original source attribution was incorrect and is not restored.
