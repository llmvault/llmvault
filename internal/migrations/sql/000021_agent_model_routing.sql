-- +goose Up
-- Model-based agent routing

ALTER TABLE agents
    ADD COLUMN available_models text[] DEFAULT '{}'::text[] NOT NULL;

UPDATE agents
SET available_models = ARRAY[model]
WHERE model IS NOT NULL
    AND model <> ''
    AND cardinality(available_models) = 0;

ALTER TABLE agents
    DROP CONSTRAINT IF EXISTS fk_agents_credential,
    DROP COLUMN IF EXISTS credential_id;

-- +goose Down
ALTER TABLE agents
    ADD COLUMN IF NOT EXISTS credential_id uuid;

CREATE INDEX IF NOT EXISTS idx_agents_credential_id ON agents USING btree (credential_id);

ALTER TABLE ONLY agents
    ADD CONSTRAINT fk_agents_credential
    FOREIGN KEY (credential_id) REFERENCES credentials(id) ON DELETE SET NULL;

ALTER TABLE agents
    DROP COLUMN IF EXISTS available_models;
