-- +goose Up
-- First-class workspace channels and channel membership

CREATE TABLE channels (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    kind text DEFAULT 'standard'::text NOT NULL,
    visibility text DEFAULT 'public'::text NOT NULL,
    default_agent_id uuid NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    origin text DEFAULT 'native'::text NOT NULL,
    external_provider text DEFAULT ''::text NOT NULL,
    external_connection_id uuid,
    external_workspace_key text DEFAULT ''::text NOT NULL,
    external_resource_type text DEFAULT 'channel'::text NOT NULL,
    external_resource_key text DEFAULT ''::text NOT NULL,
    external_resource_name text DEFAULT ''::text NOT NULL,
    external_resource_url text DEFAULT ''::text NOT NULL,
    external_metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_by uuid,
    archived_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE channel_members (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    channel_id uuid NOT NULL,
    user_id uuid NOT NULL,
    role text DEFAULT 'member'::text NOT NULL,
    created_at timestamp with time zone
);

ALTER TABLE ONLY channels ADD CONSTRAINT channels_pkey PRIMARY KEY (id);
ALTER TABLE ONLY channel_members ADD CONSTRAINT channel_members_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX idx_channels_org_name ON channels USING btree (org_id, name);
CREATE UNIQUE INDEX idx_channels_org_external_resource ON channels USING btree (org_id, external_provider, external_workspace_key, external_resource_type, external_resource_key) WHERE external_resource_key <> '';
CREATE INDEX idx_channels_default_agent_id ON channels USING btree (default_agent_id);
CREATE INDEX idx_channels_is_default ON channels USING btree (is_default);
CREATE INDEX idx_channels_origin ON channels USING btree (origin);
CREATE INDEX idx_channels_external_provider ON channels USING btree (org_id, external_provider);
CREATE INDEX idx_channels_external_connection_id ON channels USING btree (external_connection_id);
CREATE INDEX idx_channels_archived_at ON channels USING btree (archived_at);
CREATE UNIQUE INDEX idx_channel_members_channel_user ON channel_members USING btree (channel_id, user_id);
CREATE INDEX idx_channel_members_user_id ON channel_members USING btree (user_id);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'baseline down migration is intentionally unsupported; reset or restore the database instead'; END $$;
-- +goose StatementEnd
