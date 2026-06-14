-- +goose Up
-- Sandbox templates, sandboxes, warm slots, assets, domains, and microsandbox fleet

-- Sandbox, upload, and asset tables

CREATE TABLE drive_assets (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    filename text NOT NULL,
    content_type text NOT NULL,
    size bigint NOT NULL,
    s3_key text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE agent_assets (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    agent_id uuid NOT NULL,
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

CREATE TABLE sandbox_templates (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    slug text NOT NULL,
    tags jsonb DEFAULT '[]'::jsonb NOT NULL,
    size text DEFAULT 'medium'::text NOT NULL,
    base_template_id uuid,
    build_commands text DEFAULT ''::text NOT NULL,
    provider_id text DEFAULT 'daytona'::text NOT NULL,
    external_id text,
    base_image_ref text,
    build_status text DEFAULT 'pending'::text NOT NULL,
    build_error text,
    build_logs text DEFAULT ''::text NOT NULL,
    config jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE sandboxes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid,
    agent_id uuid,
    sandbox_template_id uuid,
    snapshot_id text,
    provider_id text DEFAULT 'daytona'::text NOT NULL,
    external_id text NOT NULL,
    runtime_url text NOT NULL,
    runtime_url_expires_at timestamp with time zone,
    encrypted_runtime_secret bytea NOT NULL,
    status text DEFAULT 'creating'::text NOT NULL,
    error_message text,
    last_active_at timestamp with time zone,
    stopped_at timestamp with time zone,
    memory_limit_bytes bigint DEFAULT 0 NOT NULL,
    memory_used_bytes bigint DEFAULT 0 NOT NULL,
    memory_peak_bytes bigint DEFAULT 0 NOT NULL,
    cpu_quota text DEFAULT ''::text NOT NULL,
    cpu_usage_usec bigint DEFAULT 0 NOT NULL,
    cpu_throttled_count bigint DEFAULT 0 NOT NULL,
    pid_count bigint DEFAULT 0 NOT NULL,
    resource_checked_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY drive_assets
    ADD CONSTRAINT drive_assets_pkey PRIMARY KEY (id);

ALTER TABLE ONLY agent_assets
    ADD CONSTRAINT agent_assets_pkey PRIMARY KEY (id);

ALTER TABLE ONLY sandbox_templates
    ADD CONSTRAINT sandbox_templates_pkey PRIMARY KEY (id);

ALTER TABLE ONLY sandboxes
    ADD CONSTRAINT sandboxes_pkey PRIMARY KEY (id);


CREATE INDEX idx_drive_asset_agent ON drive_assets USING btree (agent_id);

CREATE INDEX idx_drive_asset_org ON drive_assets USING btree (org_id);

CREATE UNIQUE INDEX idx_drive_assets_s3_key ON drive_assets USING btree (s3_key);

CREATE INDEX idx_emp_asset_agent_created ON agent_assets USING btree (agent_id, created_at DESC);

CREATE UNIQUE INDEX idx_agent_assets_key ON agent_assets USING btree (key);

CREATE INDEX idx_agent_assets_org_id ON agent_assets USING btree (org_id);

CREATE INDEX idx_sandbox_templates_base_template_id ON sandbox_templates USING btree (base_template_id);

CREATE INDEX idx_sandbox_templates_org_id ON sandbox_templates USING btree (org_id);

CREATE UNIQUE INDEX idx_sandbox_templates_slug ON sandbox_templates USING btree (slug);

CREATE INDEX idx_sandboxes_agent_id ON sandboxes USING btree (agent_id);

CREATE INDEX idx_sandboxes_org_id ON sandboxes USING btree (org_id);

-- Warm sandbox capacity for providers that provision slower service resources.

CREATE TABLE sandbox_warm_slots (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    provider_id text NOT NULL,
    mode text NOT NULL,
    status text DEFAULT 'warming'::text NOT NULL,
    external_id text NOT NULL,
    endpoint_url text NOT NULL,
    runtime_image text NOT NULL,
    runtime_port integer DEFAULT 7080 NOT NULL,
    region text DEFAULT ''::text NOT NULL,
    claimed_sandbox_id uuid,
    encrypted_runtime_secret bytea NOT NULL,
    error_message text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY sandbox_warm_slots
    ADD CONSTRAINT sandbox_warm_slots_pkey PRIMARY KEY (id);

ALTER TABLE ONLY sandbox_warm_slots
    ADD CONSTRAINT fk_sandbox_warm_slots_claimed_sandbox
    FOREIGN KEY (claimed_sandbox_id) REFERENCES sandboxes(id) ON DELETE SET NULL;

CREATE UNIQUE INDEX idx_sandbox_warm_slots_provider_external
    ON sandbox_warm_slots USING btree (provider_id, external_id);

CREATE INDEX idx_sandbox_warm_slots_pool_status
    ON sandbox_warm_slots USING btree (provider_id, mode, status, created_at);

CREATE INDEX idx_sandbox_warm_slots_claimed_sandbox_id
    ON sandbox_warm_slots USING btree (claimed_sandbox_id);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'baseline down migration is intentionally unsupported; reset or restore the database instead'; END $$;
-- +goose StatementEnd
