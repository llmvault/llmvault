-- +goose Up
ALTER TABLE IF EXISTS conversation_assets
    DROP CONSTRAINT IF EXISTS fk_conversation_assets_conversation;

DROP TABLE IF EXISTS conversation_assets;

-- +goose Down
CREATE TABLE IF NOT EXISTS conversation_assets (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    conversation_id uuid NOT NULL,
    org_id uuid NOT NULL,
    sandbox_id uuid NOT NULL,
    path text NOT NULL,
    filename text NOT NULL,
    key text NOT NULL,
    public_url text NOT NULL,
    content_type text NOT NULL,
    bytes bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY conversation_assets
    ADD CONSTRAINT conversation_assets_pkey PRIMARY KEY (id);

CREATE INDEX IF NOT EXISTS idx_conv_asset_conv_created ON conversation_assets USING btree (conversation_id, created_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS idx_conversation_assets_key ON conversation_assets USING btree (key);

CREATE INDEX IF NOT EXISTS idx_conversation_assets_org_id ON conversation_assets USING btree (org_id);

ALTER TABLE ONLY conversation_assets
    ADD CONSTRAINT fk_conversation_assets_conversation FOREIGN KEY (conversation_id) REFERENCES employee_sessions(id) ON DELETE CASCADE;
