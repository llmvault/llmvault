-- +goose Up
-- Agents, schedules, triggers, sessions, runtime events, and generation telemetry

-- Agent runtime, schedules, triggers, and generation tables

CREATE TABLE agent_sandbox_upgrades (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    old_sandbox_id uuid,
    new_sandbox_id uuid,
    status character varying(32) DEFAULT 'queued'::character varying NOT NULL,
    phase character varying(64) DEFAULT 'queued'::character varying NOT NULL,
    backup_key text,
    backup_sha256 text,
    backup_bytes bigint DEFAULT 0 NOT NULL,
    error_message text,
    completed_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE agent_schedule_runs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    schedule_id uuid NOT NULL,
    sandbox_id uuid,
    runtime_job_id character varying(255) NOT NULL,
    run_key character varying(500) NOT NULL,
    status character varying(64) DEFAULT 'running'::character varying NOT NULL,
    scheduled_at timestamp with time zone,
    started_at timestamp with time zone,
    completed_at timestamp with time zone,
    duration_ms bigint,
    error text DEFAULT ''::text NOT NULL,
    event_payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE agent_schedules (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    sandbox_id uuid,
    runtime_job_id character varying(255) NOT NULL,
    status character varying(64) DEFAULT 'active'::character varying NOT NULL,
    channel character varying(255) DEFAULT ''::character varying NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    task_prompt text DEFAULT ''::text NOT NULL,
    interval_seconds bigint,
    repeat_count bigint,
    repeat_completed bigint DEFAULT 0 NOT NULL,
    next_run_at timestamp with time zone,
    last_run_at timestamp with time zone,
    last_status character varying(64) DEFAULT ''::character varying NOT NULL,
    last_error text DEFAULT ''::text NOT NULL,
    created_by_session character varying(255) DEFAULT ''::character varying NOT NULL,
    runtime_created_at timestamp with time zone,
    cancelled_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE agent_skills (
    agent_id uuid NOT NULL,
    skill_id uuid NOT NULL,
    created_at timestamp with time zone
);

CREATE TABLE agent_trigger_deliveries (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    trigger_id uuid NOT NULL,
    connection_id uuid,
    delivery_id text NOT NULL,
    event_key text DEFAULT ''::text NOT NULL,
    resource_key text DEFAULT ''::text NOT NULL,
    session_id uuid NOT NULL,
    runtime_session_id text DEFAULT ''::text NOT NULL,
    runtime_stream_id text DEFAULT ''::text NOT NULL,
    runtime_trace_id text DEFAULT ''::text NOT NULL,
    runtime_turn_id text DEFAULT ''::text NOT NULL,
    payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone
);

CREATE TABLE agent_triggers (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    trigger_type character varying(32) DEFAULT 'webhook'::character varying NOT NULL,
    connection_id uuid,
    trigger_keys text[] DEFAULT '{}'::text[] NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    conditions jsonb,
    instructions text DEFAULT ''::text NOT NULL,
    secret_key text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE agents (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    avatar_url text,
    icon text DEFAULT ''::text NOT NULL,
    placeholder text DEFAULT ''::text NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    sandbox_strategy text DEFAULT 'per_session'::text NOT NULL,
    workspace_snapshot_id uuid,
    credential_id uuid,
    sandbox_template_id uuid,
    instructions text,
    model text NOT NULL,
    tools jsonb DEFAULT '{}'::jsonb NOT NULL,
    mcp_servers jsonb DEFAULT '[]'::jsonb NOT NULL,
    skills jsonb DEFAULT '{}'::jsonb NOT NULL,
    runtime_config jsonb DEFAULT '{}'::jsonb NOT NULL,
    permissions jsonb DEFAULT '{}'::jsonb NOT NULL,
    resources jsonb DEFAULT '{}'::jsonb NOT NULL,
    shared_memory boolean DEFAULT true NOT NULL,
    sandbox_tools text[] DEFAULT '{}'::text[],
    setup_commands text[] DEFAULT '{}'::text[],
    encrypted_env_vars bytea,
    status text DEFAULT 'active'::text NOT NULL,
    last_memory_refreshed_at timestamp with time zone,
    memory_refresh_status character varying(32) DEFAULT ''::character varying NOT NULL,
    memory_refresh_error text DEFAULT ''::text NOT NULL,
    last_proxy_token_refreshed_at timestamp with time zone,
    harness character varying(32) DEFAULT ''::character varying NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE failed_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    trigger_id uuid NOT NULL,
    event_type text NOT NULL,
    payload jsonb NOT NULL,
    error text NOT NULL,
    attempt_count bigint NOT NULL,
    failed_at timestamp with time zone NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    retried_at timestamp with time zone,
    retried_task_id text
);

CREATE TABLE generations (
    id text NOT NULL,
    org_id uuid NOT NULL,
    credential_id uuid NOT NULL,
    token_jti text NOT NULL,
    provider_id text NOT NULL,
    model text,
    request_path text,
    is_streaming boolean DEFAULT false,
    input_tokens bigint DEFAULT 0,
    output_tokens bigint DEFAULT 0,
    cached_tokens bigint DEFAULT 0,
    reasoning_tokens bigint DEFAULT 0,
    cost numeric(12,8) DEFAULT 0,
    ttfb_ms bigint,
    total_ms bigint,
    upstream_status bigint,
    user_id text,
    tags text[],
    error_type text,
    error_message text,
    ip_address inet,
    created_at timestamp with time zone NOT NULL,
    is_system boolean DEFAULT false NOT NULL,
    billed_at timestamp with time zone,
    billing_error text,
    credits_debited bigint DEFAULT 0 NOT NULL,
    billing_cost_source text DEFAULT ''::text NOT NULL
);

CREATE TABLE hindsight_banks (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    agent_id uuid,
    bank_id text NOT NULL,
    config_hash text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY agent_sandbox_upgrades
    ADD CONSTRAINT agent_sandbox_upgrades_pkey PRIMARY KEY (id);

ALTER TABLE ONLY agent_schedule_runs
    ADD CONSTRAINT agent_schedule_runs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY agent_schedules
    ADD CONSTRAINT agent_schedules_pkey PRIMARY KEY (id);

ALTER TABLE ONLY agent_skills
    ADD CONSTRAINT agent_skills_pkey PRIMARY KEY (agent_id, skill_id);

ALTER TABLE ONLY agent_trigger_deliveries
    ADD CONSTRAINT agent_trigger_deliveries_pkey PRIMARY KEY (id);

ALTER TABLE ONLY agent_triggers
    ADD CONSTRAINT agent_triggers_pkey PRIMARY KEY (id);

ALTER TABLE ONLY agents
    ADD CONSTRAINT agents_pkey PRIMARY KEY (id);

ALTER TABLE ONLY failed_events
    ADD CONSTRAINT failed_events_pkey PRIMARY KEY (id);

ALTER TABLE ONLY generations
    ADD CONSTRAINT generations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY hindsight_banks
    ADD CONSTRAINT hindsight_banks_pkey PRIMARY KEY (id);

CREATE INDEX idx_agent_org_id ON agents USING btree (org_id);

CREATE UNIQUE INDEX idx_agents_org_name ON agents USING btree (org_id, name);

CREATE INDEX idx_agents_is_default ON agents USING btree (is_default);

CREATE INDEX idx_agent_sandbox_upgrades_agent_id ON agent_sandbox_upgrades USING btree (agent_id);

CREATE INDEX idx_agent_sandbox_upgrades_new_sandbox_id ON agent_sandbox_upgrades USING btree (new_sandbox_id);

CREATE INDEX idx_agent_sandbox_upgrades_old_sandbox_id ON agent_sandbox_upgrades USING btree (old_sandbox_id);

CREATE INDEX idx_agent_sandbox_upgrades_org_id ON agent_sandbox_upgrades USING btree (org_id);

CREATE INDEX idx_agent_sandbox_upgrades_status ON agent_sandbox_upgrades USING btree (status);

CREATE UNIQUE INDEX idx_agent_schedule_agent_runtime ON agent_schedules USING btree (agent_id, runtime_job_id);

CREATE UNIQUE INDEX idx_agent_schedule_run_key ON agent_schedule_runs USING btree (schedule_id, run_key);

CREATE INDEX idx_agent_schedule_runs_runtime_job_id ON agent_schedule_runs USING btree (runtime_job_id);

CREATE INDEX idx_agent_schedule_runs_agent_id ON agent_schedule_runs USING btree (agent_id);

CREATE INDEX idx_agent_schedule_runs_org_id ON agent_schedule_runs USING btree (org_id);

CREATE INDEX idx_agent_schedule_runs_sandbox_id ON agent_schedule_runs USING btree (sandbox_id);

CREATE INDEX idx_agent_schedule_runs_scheduled_at ON agent_schedule_runs USING btree (scheduled_at);

CREATE INDEX idx_agent_schedule_runs_status ON agent_schedule_runs USING btree (status);

CREATE INDEX idx_agent_schedules_cancelled_at ON agent_schedules USING btree (cancelled_at);

CREATE INDEX idx_agent_schedules_next_run_at ON agent_schedules USING btree (next_run_at);

CREATE INDEX idx_agent_schedules_org_id ON agent_schedules USING btree (org_id);

CREATE INDEX idx_agent_schedules_sandbox_id ON agent_schedules USING btree (sandbox_id);

CREATE INDEX idx_agent_schedules_status ON agent_schedules USING btree (status);

CREATE INDEX idx_agent_trigger_deliveries_connection_id ON agent_trigger_deliveries USING btree (connection_id);

CREATE INDEX idx_agent_trigger_deliveries_session_id ON agent_trigger_deliveries USING btree (session_id);

CREATE INDEX idx_agent_trigger_deliveries_delivery_id ON agent_trigger_deliveries USING btree (delivery_id);

CREATE INDEX idx_agent_trigger_deliveries_event_key ON agent_trigger_deliveries USING btree (event_key);

CREATE INDEX idx_agent_trigger_deliveries_resource_key ON agent_trigger_deliveries USING btree (resource_key);

CREATE INDEX idx_agent_trigger_deliveries_runtime_session_id ON agent_trigger_deliveries USING btree (runtime_session_id);

CREATE INDEX idx_agent_trigger_deliveries_trigger_id ON agent_trigger_deliveries USING btree (trigger_id);

CREATE INDEX idx_agent_triggers_connection_id ON agent_triggers USING btree (connection_id);

CREATE INDEX idx_agent_triggers_agent_id ON agent_triggers USING btree (agent_id);

CREATE INDEX idx_agent_triggers_org_id ON agent_triggers USING btree (org_id);

CREATE INDEX idx_agent_triggers_trigger_type ON agent_triggers USING btree (trigger_type);

CREATE INDEX idx_agents_credential_id ON agents USING btree (credential_id);

CREATE INDEX idx_failed_events_event_type ON failed_events USING btree (event_type);

CREATE INDEX idx_failed_events_failed_at ON failed_events USING btree (failed_at);

CREATE INDEX idx_failed_events_org_id ON failed_events USING btree (org_id);

CREATE INDEX idx_failed_events_status ON failed_events USING btree (status);

CREATE INDEX idx_failed_events_trigger_id ON failed_events USING btree (trigger_id);

CREATE INDEX idx_gen_org_created ON generations USING btree (org_id, created_at);

CREATE INDEX idx_gen_org_credential ON generations USING btree (credential_id);

CREATE INDEX idx_gen_org_model ON generations USING btree (model);

CREATE INDEX idx_gen_org_provider ON generations USING btree (provider_id);

CREATE INDEX idx_gen_org_user ON generations USING btree (user_id);

CREATE UNIQUE INDEX idx_hindsight_banks_bank_id ON hindsight_banks USING btree (bank_id);

CREATE INDEX idx_hindsight_banks_agent_id ON hindsight_banks USING btree (agent_id);

CREATE INDEX idx_trigger_delivery_org_agent_created ON agent_trigger_deliveries USING btree (org_id, agent_id, created_at);

CREATE INDEX idx_trigger_delivery_org_agent_session_created ON agent_trigger_deliveries USING btree (org_id, agent_id, runtime_session_id, created_at);

ALTER TABLE agent_schedules
    ADD COLUMN is_system boolean NOT NULL DEFAULT false,
    ADD COLUMN provider text NOT NULL DEFAULT '',
    ADD COLUMN connection_id uuid,
    ADD COLUMN metadata jsonb NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX idx_agent_schedules_is_system ON agent_schedules USING btree (is_system);
CREATE INDEX idx_agent_schedules_provider ON agent_schedules USING btree (provider);
CREATE INDEX idx_agent_schedules_connection_id ON agent_schedules USING btree (connection_id);

ALTER TABLE ONLY agent_schedules
    ADD CONSTRAINT fk_agent_schedules_connection FOREIGN KEY (connection_id) REFERENCES connections(id) ON DELETE SET NULL;

-- Track how many times the billing batch has attempted (and failed) to debit a
-- generation row. Rows that fail with insufficient_credits are left with
-- billed_at NULL so the existing sweep retries them after a top-up; this counter
-- caps the retries so a permanently underfunded org cannot hot-loop the batch.
ALTER TABLE generations
    ADD COLUMN IF NOT EXISTS billing_attempts integer DEFAULT 0 NOT NULL;

-- Supporting indexes for the billing batch tick (P2-44).
--
-- 1) Unbilled scan. selectUnbilledBatch does
--      WHERE billed_at IS NULL AND is_system = TRUE
--        AND (cost > 0 OR input_tokens > 0 OR output_tokens > 0)
--      ORDER BY created_at
--      LIMIT n FOR UPDATE OF g SKIP LOCKED
--    Without a supporting index this seq-scans the full generations table on
--    every tick and re-sorts by created_at. A partial index over only the
--    unbilled system rows, ordered by created_at, keeps the scan proportional to
--    the (small) backlog rather than total history, and serves the ORDER BY/LIMIT
--    directly. The set this index covers shrinks back toward empty as the batch
--    drains the queue.
CREATE INDEX IF NOT EXISTS idx_gen_unbilled_system_created
    ON generations (created_at)
    WHERE billed_at IS NULL AND is_system = TRUE;

-- 2) Per-org billed-cost sum. planCumulativeDebits recomputes
--      SUM(cost) WHERE org_id = ? AND is_system = TRUE
--        AND billed_at IS NOT NULL AND billing_error = '' AND cost > 0
--    per org each tick to keep split/concurrent ticks idempotent (the debit is
--    cumulative, not per-row). This is still O(billed history per org); a true
--    fix is an incremental per-org counter, deferred as too invasive for a P2
--    cleanup (it would change the cumulative-debit idempotency contract the
--    concurrent-tick tests rely on). This partial index at least bounds the sum
--    to the matching rows of one org via an index scan instead of a heap scan of
--    the whole table.
CREATE INDEX IF NOT EXISTS idx_gen_billed_org_cost
    ON generations (org_id, cost)
    WHERE is_system = TRUE AND billed_at IS NOT NULL AND billing_error = '' AND cost > 0;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'baseline down migration is intentionally unsupported; reset or restore the database instead'; END $$;
-- +goose StatementEnd
