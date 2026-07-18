-- +goose Up

CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;
CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public;
CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public;

-- +goose StatementBegin
CREATE FUNCTION public.bump_mcp_config_version() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    target_org_id uuid;
BEGIN
    target_org_id := CASE WHEN TG_OP = 'DELETE' THEN OLD.org_id ELSE NEW.org_id END;
    UPDATE public.orgs
       SET mcp_config_version = mcp_config_version + 1
     WHERE id = target_org_id;
    RETURN CASE WHEN TG_OP = 'DELETE' THEN OLD ELSE NEW END;
END;
$$;
-- +goose StatementEnd

CREATE TABLE public.agent_assets (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    sandbox_id uuid,
    path text NOT NULL,
    filename text NOT NULL,
    key text NOT NULL,
    public_url text NOT NULL,
    content_type text NOT NULL,
    bytes bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    description jsonb
);

CREATE TABLE public.agent_catalog (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    slug text NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    category text DEFAULT ''::text NOT NULL,
    avatar_url text DEFAULT ''::text NOT NULL,
    developer text DEFAULT 'Hivy'::text NOT NULL,
    official boolean DEFAULT false NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    model text DEFAULT ''::text NOT NULL,
    instructions text DEFAULT ''::text NOT NULL,
    required_connections text[] DEFAULT '{}'::text[] NOT NULL,
    manifest jsonb DEFAULT '{}'::jsonb NOT NULL,
    source_hash text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    sub_agents jsonb DEFAULT '{}'::jsonb NOT NULL,
    sandbox_image text DEFAULT 'default'::text NOT NULL,
    tools jsonb DEFAULT '{}'::jsonb NOT NULL,
    default_reasoning_effort text DEFAULT ''::text NOT NULL,
    auto_load_skills jsonb DEFAULT '[]'::jsonb NOT NULL,
    CONSTRAINT agent_catalog_sandbox_image_valid CHECK ((sandbox_image = ANY (ARRAY['default'::text, 'developer'::text])))
);

CREATE TABLE public.agent_directives (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    content text NOT NULL,
    created_by_user_id uuid,
    source text DEFAULT 'user-pinned'::text NOT NULL,
    active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    CONSTRAINT agent_directives_source_check CHECK ((source = ANY (ARRAY['user-pinned'::text, 'extracted-confirmed'::text])))
);

CREATE TABLE public.agent_mcp_servers (
    org_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    mcp_server_id uuid NOT NULL,
    state text NOT NULL,
    updated_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT agent_mcp_servers_state_check CHECK ((state = ANY (ARRAY['enabled'::text, 'disabled'::text])))
);

CREATE TABLE public.agent_memories (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    content text NOT NULL,
    tags text[] DEFAULT '{}'::text[] NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    embedding public.vector(1024),
    embedding_model text DEFAULT ''::text NOT NULL,
    embedding_status text DEFAULT 'pending'::text NOT NULL,
    embedding_revision integer DEFAULT 1 NOT NULL,
    embedding_error text DEFAULT ''::text NOT NULL,
    embedded_at timestamp with time zone,
    source_session_id uuid,
    source_event_id uuid,
    created_by_user_id uuid,
    archived_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    memory_fingerprint text DEFAULT ''::text NOT NULL,
    consolidated_at timestamp with time zone,
    CONSTRAINT agent_memories_embedding_status_check CHECK ((embedding_status = ANY (ARRAY['pending'::text, 'ready'::text, 'failed'::text])))
);

CREATE TABLE public.agent_observations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    content text NOT NULL,
    kind text NOT NULL,
    entities text[] DEFAULT '{}'::text[] NOT NULL,
    proof_count integer DEFAULT 1 NOT NULL,
    source_fact_ids uuid[] DEFAULT '{}'::uuid[] NOT NULL,
    occurred_start timestamp with time zone,
    occurred_end timestamp with time zone,
    last_mentioned_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone,
    superseded_by uuid,
    archived_at timestamp with time zone,
    human_verified boolean DEFAULT false NOT NULL,
    embedding public.vector(1024),
    embedding_model text DEFAULT ''::text NOT NULL,
    embedding_status text DEFAULT 'pending'::text NOT NULL,
    embedding_revision integer DEFAULT 1 NOT NULL,
    embedding_error text DEFAULT ''::text NOT NULL,
    embedded_at timestamp with time zone,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT agent_observations_embedding_status_check CHECK ((embedding_status = ANY (ARRAY['pending'::text, 'ready'::text, 'failed'::text])))
);

CREATE TABLE public.agent_schedule_runs (
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
    updated_at timestamp with time zone,
    session_id uuid,
    lease_owner text DEFAULT ''::text NOT NULL,
    leased_until timestamp with time zone
);

CREATE TABLE public.agent_schedules (
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
    updated_at timestamp with time zone,
    is_system boolean DEFAULT false NOT NULL,
    provider text DEFAULT ''::text NOT NULL,
    connection_id uuid,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    schedule_kind text DEFAULT 'interval'::text NOT NULL,
    cron_expression text,
    lease_owner text DEFAULT ''::text NOT NULL,
    leased_until timestamp with time zone,
    source_slug text DEFAULT ''::text NOT NULL,
    name text,
    created_by_user_id uuid
);

CREATE TABLE public.agent_trigger_deliveries (
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

CREATE TABLE public.agent_triggers (
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
    updated_at timestamp with time zone,
    source_slug text DEFAULT ''::text NOT NULL,
    resource_type text DEFAULT ''::text NOT NULL,
    resource_key text DEFAULT ''::text NOT NULL,
    resource_name text DEFAULT ''::text NOT NULL,
    trigger_key text DEFAULT ''::text NOT NULL,
    trigger_value text DEFAULT ''::text NOT NULL,
    name text
);

CREATE TABLE public.agents (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    avatar_url text,
    icon text DEFAULT ''::text NOT NULL,
    placeholder text DEFAULT ''::text NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    workspace_snapshot_id uuid,
    sandbox_template_id uuid,
    instructions text,
    model text NOT NULL,
    tools jsonb DEFAULT '{}'::jsonb NOT NULL,
    mcp_servers jsonb DEFAULT '[]'::jsonb NOT NULL,
    skills jsonb DEFAULT '{}'::jsonb NOT NULL,
    runtime_config jsonb DEFAULT '{}'::jsonb NOT NULL,
    permissions jsonb DEFAULT '{}'::jsonb NOT NULL,
    resources jsonb DEFAULT '{}'::jsonb NOT NULL,
    sandbox_tools text[] DEFAULT '{}'::text[],
    setup_commands text[] DEFAULT '{}'::text[],
    status text DEFAULT 'active'::text NOT NULL,
    last_proxy_token_refreshed_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    agent_catalog_id uuid,
    sandbox_size text DEFAULT 'small'::text NOT NULL,
    sandbox_image text DEFAULT 'default'::text NOT NULL,
    image_model text DEFAULT ''::text NOT NULL,
    vector_image_model text DEFAULT ''::text NOT NULL,
    type text DEFAULT 'agent'::text NOT NULL,
    parent_agent_id uuid,
    mcp_tool_filter jsonb,
    default_reasoning_effort text,
    auto_load_skills jsonb DEFAULT '[]'::jsonb NOT NULL,
    team_id uuid NOT NULL,
    instructions_snapshot text,
    connection_mcp_tool_deny jsonb DEFAULT '{}'::jsonb NOT NULL,
    email_inbox_local_part text DEFAULT ''::text NOT NULL,
    memory_mission text,
    CONSTRAINT agents_sandbox_image_valid CHECK ((sandbox_image = ANY (ARRAY['default'::text, 'developer'::text]))),
    CONSTRAINT agents_sandbox_size_check CHECK ((sandbox_size = ANY (ARRAY['nano'::text, 'small'::text, 'medium'::text, 'large'::text, 'xlarge'::text]))),
    CONSTRAINT agents_type_check CHECK ((type = ANY (ARRAY['agent'::text, 'subagent'::text])))
);

CREATE TABLE public.agent_memory_digests (
    agent_id uuid NOT NULL,
    org_id uuid NOT NULL,
    content text NOT NULL,
    observation_count integer DEFAULT 0 NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.agent_email_threads (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    session_id uuid,
    root_message_id text DEFAULT ''::text NOT NULL,
    reply_token text NOT NULL,
    last_message_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.agent_email_messages (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    thread_id uuid NOT NULL,
    direction text NOT NULL,
    status text DEFAULT 'received'::text NOT NULL,
    resend_email_id text DEFAULT ''::text NOT NULL,
    message_id text DEFAULT ''::text NOT NULL,
    in_reply_to text DEFAULT ''::text NOT NULL,
    "references" jsonb DEFAULT '[]'::jsonb NOT NULL,
    from_address text DEFAULT ''::text NOT NULL,
    to_addresses jsonb DEFAULT '[]'::jsonb NOT NULL,
    cc_addresses jsonb DEFAULT '[]'::jsonb NOT NULL,
    subject text DEFAULT ''::text NOT NULL,
    text_body text DEFAULT ''::text NOT NULL,
    html_body text DEFAULT ''::text NOT NULL,
    headers jsonb DEFAULT '{}'::jsonb NOT NULL,
    provider_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.agent_email_webhook_receipts (
    svix_id text NOT NULL,
    event_type text NOT NULL,
    resend_email_id text DEFAULT ''::text NOT NULL,
    payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    processed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.api_keys (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    name text NOT NULL,
    key_hash text NOT NULL,
    key_prefix text NOT NULL,
    scopes text[] NOT NULL,
    expires_at timestamp with time zone,
    last_used_at timestamp with time zone,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone,
    created_by uuid
);

CREATE TABLE public.app_versions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    app_id uuid NOT NULL,
    org_id uuid NOT NULL,
    source_object_key text NOT NULL,
    bundle_object_key text NOT NULL,
    source_sha256 text NOT NULL,
    bundle_sha256 text NOT NULL,
    source_bytes bigint DEFAULT 0 NOT NULL,
    bundle_bytes bigint DEFAULT 0 NOT NULL,
    notes text DEFAULT ''::text NOT NULL,
    template_version text DEFAULT ''::text NOT NULL,
    created_by_agent_id uuid,
    source_session_id uuid,
    archived_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.apps (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    team_id uuid NOT NULL,
    sheet_id uuid NOT NULL,
    slug text NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    icon text DEFAULT ''::text NOT NULL,
    alias text DEFAULT ''::text NOT NULL,
    sandbox_id uuid,
    encrypted_app_secret bytea NOT NULL,
    active_version_id uuid,
    status text DEFAULT 'draft'::text NOT NULL,
    template_version text DEFAULT ''::text NOT NULL,
    created_by_agent_id uuid,
    created_by_user_id uuid,
    source_session_id uuid,
    archived_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    preview_url text DEFAULT ''::text NOT NULL,
    preview_registered_at timestamp with time zone,
    alias_url text DEFAULT ''::text NOT NULL,
    CONSTRAINT apps_status_check CHECK ((status = ANY (ARRAY['draft'::text, 'deploying'::text, 'running'::text, 'stopped'::text, 'failed'::text])))
);

CREATE TABLE public.audit_log (
    id bigint NOT NULL,
    org_id uuid NOT NULL,
    credential_id uuid,
    action text NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb,
    ip_address inet,
    created_at timestamp with time zone
);

CREATE SEQUENCE public.audit_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.audit_log_id_seq OWNED BY public.audit_log.id;

CREATE TABLE public.brand_assets (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    brand_id uuid NOT NULL,
    kind text NOT NULL,
    role text DEFAULT ''::text NOT NULL,
    name text NOT NULL,
    key text NOT NULL,
    public_url text NOT NULL,
    content_type text NOT NULL,
    bytes bigint NOT NULL,
    width integer,
    height integer,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_by uuid,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    CONSTRAINT brand_assets_bytes_check CHECK ((bytes > 0)),
    CONSTRAINT brand_assets_height_check CHECK (((height IS NULL) OR (height > 0))),
    CONSTRAINT brand_assets_kind_check CHECK ((kind = ANY (ARRAY['logo'::text, 'mark'::text, 'icon'::text, 'image'::text, 'font'::text, 'document'::text, 'other'::text]))),
    CONSTRAINT brand_assets_width_check CHECK (((width IS NULL) OR (width > 0)))
);

CREATE TABLE public.brands (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    name text NOT NULL,
    slug text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    logos jsonb DEFAULT '{}'::jsonb NOT NULL,
    colors jsonb DEFAULT '{}'::jsonb NOT NULL,
    typography jsonb DEFAULT '{}'::jsonb NOT NULL,
    voice jsonb DEFAULT '{}'::jsonb NOT NULL,
    source jsonb DEFAULT '{"origin": "manual", "version": 1}'::jsonb NOT NULL,
    raw_import jsonb,
    archived_at timestamp with time zone,
    created_by uuid,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE public.canvas_artifact_files (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    canvas_artifact_id uuid NOT NULL,
    path text NOT NULL,
    role text DEFAULT ''::text NOT NULL,
    content_type text DEFAULT ''::text NOT NULL,
    object_key text NOT NULL,
    size_bytes bigint DEFAULT 0 NOT NULL,
    sha256 text DEFAULT ''::text NOT NULL,
    archived_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT canvas_artifact_files_size_check CHECK ((size_bytes >= 0))
);

CREATE TABLE public.canvas_artifacts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    canvas_project_id uuid NOT NULL,
    slug text NOT NULL,
    type text NOT NULL,
    name text DEFAULT ''::text NOT NULL,
    manifest_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    source_session_id uuid,
    created_by_agent_id uuid,
    created_by_user_id uuid,
    archived_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT canvas_artifacts_type_check CHECK ((type = ANY (ARRAY['web_page'::text, 'presentation'::text])))
);

CREATE TABLE public.canvas_projects (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    name text DEFAULT ''::text NOT NULL,
    created_by_agent_id uuid,
    created_by_user_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    slug text DEFAULT ''::text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    archived_at timestamp with time zone
);

CREATE TABLE public.connections (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid,
    user_id uuid NOT NULL,
    integration_id uuid NOT NULL,
    nango_connection_id text NOT NULL,
    meta jsonb DEFAULT '{}'::jsonb,
    webhook_configured boolean DEFAULT true NOT NULL,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    name text DEFAULT ''::text NOT NULL,
    slug text DEFAULT ''::text NOT NULL,
    needs_name boolean DEFAULT false NOT NULL
);

CREATE TABLE public.team_external_resource_routes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    team_id uuid NOT NULL,
    connection_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    resource_type text NOT NULL,
    resource_key text NOT NULL,
    resource_name text DEFAULT ''::text NOT NULL,
    resource_url text DEFAULT ''::text NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.credentials (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid,
    label text DEFAULT ''::text NOT NULL,
    base_url text NOT NULL,
    auth_scheme text NOT NULL,
    encrypted_key bytea NOT NULL,
    wrapped_dek bytea NOT NULL,
    remaining bigint,
    refill_amount bigint,
    refill_interval text,
    last_refill_at timestamp with time zone,
    provider_id text DEFAULT ''::text,
    meta jsonb DEFAULT '{}'::jsonb,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone
);

CREATE TABLE public.credit_ledger_entries (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    amount bigint NOT NULL,
    reason character varying(64) NOT NULL,
    ref_type character varying(64),
    ref_id character varying(64),
    created_at timestamp with time zone
);

CREATE TABLE public.database_connections (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    provider character varying(32) NOT NULL,
    display_name text DEFAULT ''::text NOT NULL,
    encrypted_dsn bytea NOT NULL,
    wrapped_dek bytea NOT NULL,
    schema_snapshot jsonb DEFAULT '{}'::jsonb NOT NULL,
    access_policy jsonb DEFAULT '{}'::jsonb NOT NULL,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    name text DEFAULT ''::text NOT NULL,
    slug text DEFAULT ''::text NOT NULL,
    needs_name boolean DEFAULT false NOT NULL
);

CREATE TABLE public.drive_assets (
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

CREATE TABLE public.email_verifications (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    token_hash text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    used_at timestamp with time zone,
    created_at timestamp with time zone
);

CREATE TABLE public.generations (
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
    billing_cost_source text DEFAULT ''::text NOT NULL,
    billing_attempts integer DEFAULT 0 NOT NULL,
    openrouter_generation_id text,
    session_id uuid
);

CREATE TABLE public.github_pull_request_sessions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    agent_id uuid,
    repo text NOT NULL,
    pr_number integer NOT NULL,
    session_id uuid NOT NULL,
    head_ref text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.integrations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    unique_key text NOT NULL,
    provider text NOT NULL,
    display_name text NOT NULL,
    meta jsonb DEFAULT '{}'::jsonb,
    nango_config jsonb DEFAULT '{}'::jsonb,
    supports_rag_source boolean DEFAULT false NOT NULL,
    deleted_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    bot_handle text DEFAULT ''::text NOT NULL
);

CREATE TABLE public.mcp_authorizations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    mcp_server_id uuid NOT NULL,
    principal_type text NOT NULL,
    principal_user_id uuid,
    auth_type text NOT NULL,
    credentials_encrypted bytea NOT NULL,
    client_id text DEFAULT ''::text NOT NULL,
    scopes text[] DEFAULT '{}'::text[] NOT NULL,
    token_type text DEFAULT ''::text NOT NULL,
    expires_at timestamp with time zone,
    refresh_expires_at timestamp with time zone,
    status text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT mcp_authorizations_auth_type_check CHECK ((auth_type = ANY (ARRAY['none'::text, 'static_bearer'::text, 'static_header'::text, 'oauth_authorization_code'::text, 'oauth_client_credentials'::text]))),
    CONSTRAINT mcp_authorizations_principal_check CHECK ((((principal_type = 'user'::text) AND (principal_user_id IS NOT NULL)) OR ((principal_type = 'org_service'::text) AND (principal_user_id IS NULL)))),
    CONSTRAINT mcp_authorizations_status_check CHECK ((status = ANY (ARRAY['active'::text, 'expired'::text, 'revoked'::text])))
);

CREATE TABLE public.mcp_oauth_states (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    mcp_server_id uuid NOT NULL,
    user_id uuid NOT NULL,
    principal_type text NOT NULL,
    state_hash bytea NOT NULL,
    encrypted_verifier bytea NOT NULL,
    redirect_after text DEFAULT ''::text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    used_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT mcp_oauth_states_principal_type_check CHECK ((principal_type = ANY (ARRAY['user'::text, 'org_service'::text])))
);

CREATE TABLE public.mcp_servers (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    scope text NOT NULL,
    owner_user_id uuid,
    slug text NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    url text NOT NULL,
    transport text DEFAULT 'streamable_http'::text NOT NULL,
    auth_type text DEFAULT 'none'::text NOT NULL,
    authorization_policy text DEFAULT 'none'::text NOT NULL,
    header_name text DEFAULT ''::text NOT NULL,
    oauth_metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    created_by_user_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT mcp_servers_auth_type_check CHECK ((auth_type = ANY (ARRAY['none'::text, 'static_bearer'::text, 'static_header'::text, 'oauth_authorization_code'::text, 'oauth_client_credentials'::text]))),
    CONSTRAINT mcp_servers_authorization_policy_check CHECK ((authorization_policy = ANY (ARRAY['none'::text, 'user_required'::text, 'service_required'::text, 'prefer_user'::text, 'prefer_service'::text]))),
    CONSTRAINT mcp_servers_scope_check CHECK ((((scope = 'personal'::text) AND (owner_user_id IS NOT NULL)) OR ((scope = 'org'::text) AND (owner_user_id IS NULL)))),
    CONSTRAINT mcp_servers_status_check CHECK ((status = ANY (ARRAY['active'::text, 'disabled'::text]))),
    CONSTRAINT mcp_servers_transport_check CHECK ((transport = ANY (ARRAY['streamable_http'::text, 'sse'::text])))
);

CREATE TABLE public.memory_suppressions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    content_fingerprint text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.oauth_accounts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    provider text NOT NULL,
    provider_user_id text NOT NULL,
    provider_user_email text,
    provider_user_login text,
    verified_emails text[],
    last_synced_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE public.oauth_exchange_tokens (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    token_hash text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    used_at timestamp with time zone,
    created_at timestamp with time zone
);

CREATE TABLE public.org_invite_teams (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    org_invite_id uuid NOT NULL,
    team_id uuid NOT NULL,
    created_at timestamp with time zone
);

CREATE TABLE public.org_invites (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    email text NOT NULL,
    role text NOT NULL,
    token_hash text NOT NULL,
    invited_by_id uuid NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    accepted_at timestamp with time zone,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE public.org_memberships (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    org_id uuid NOT NULL,
    role text DEFAULT 'owner'::text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deactivated_at timestamp with time zone
);

CREATE TABLE public.orgs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    rate_limit bigint DEFAULT 1000 NOT NULL,
    active boolean DEFAULT true NOT NULL,
    allowed_origins text[],
    byok boolean DEFAULT false NOT NULL,
    logo_url text DEFAULT ''::text NOT NULL,
    website character varying(500) DEFAULT ''::character varying NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    prompt_company text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    sandbox_exposed_ports integer[] DEFAULT '{3000,5173,8000,8080}'::integer[] NOT NULL,
    onboarding_step text DEFAULT 'complete'::text NOT NULL,
    mcp_config_version bigint DEFAULT 0 NOT NULL,
    billing_currency character varying(3) DEFAULT ''::character varying NOT NULL,
    CONSTRAINT orgs_billing_currency_check CHECK (((billing_currency)::text = ANY ((ARRAY[''::character varying, 'USD'::character varying, 'NGN'::character varying])::text[]))),
    CONSTRAINT orgs_onboarding_step_check CHECK ((onboarding_step = ANY (ARRAY['team'::text, 'connections'::text, 'welcome'::text, 'complete'::text])))
);

CREATE TABLE public.billing_payment_methods (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    user_id uuid NOT NULL,
    provider character varying(32) NOT NULL,
    provider_signature character varying(128) NOT NULL,
    encrypted_authorization bytea NOT NULL,
    wrapped_dek bytea NOT NULL,
    card_type character varying(32) DEFAULT ''::character varying NOT NULL,
    last4 character varying(4) DEFAULT ''::character varying NOT NULL,
    exp_month character varying(2) DEFAULT ''::character varying NOT NULL,
    exp_year character varying(4) DEFAULT ''::character varying NOT NULL,
    bank character varying(128) DEFAULT ''::character varying NOT NULL,
    country_code character varying(2) DEFAULT ''::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.credit_purchases (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    created_by_user_id uuid,
    pack_id character varying(32) NOT NULL,
    idempotency_key character varying(64) NOT NULL,
    payment_method_id uuid,
    save_payment_method boolean DEFAULT false NOT NULL,
    provider character varying(32) NOT NULL,
    provider_reference character varying(128) DEFAULT ''::character varying NOT NULL,
    checkout_access_code character varying(128) DEFAULT ''::character varying NOT NULL,
    checkout_url text DEFAULT ''::text NOT NULL,
    status character varying(32) DEFAULT 'pending'::character varying NOT NULL,
    currency character varying(3) NOT NULL,
    subtotal_minor bigint NOT NULL,
    fee_basis_points bigint NOT NULL,
    fee_minor bigint NOT NULL,
    total_minor bigint NOT NULL,
    credits bigint NOT NULL,
    fx_minor_per_usd bigint,
    provider_paid_minor bigint DEFAULT 0 NOT NULL,
    provider_paid_currency character varying(3) DEFAULT ''::character varying NOT NULL,
    paid_at timestamp with time zone,
    credited_at timestamp with time zone,
    failed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT credit_purchases_currency_check CHECK (((currency)::text = ANY ((ARRAY['USD'::character varying, 'NGN'::character varying])::text[]))),
    CONSTRAINT credit_purchases_status_check CHECK (((status)::text = ANY ((ARRAY['pending'::character varying, 'paid'::character varying, 'credited'::character varying, 'failed'::character varying, 'reversed'::character varying, 'refunded'::character varying])::text[]))),
    CONSTRAINT credit_purchases_amounts_check CHECK (((subtotal_minor > 0) AND (fee_minor >= 0) AND (total_minor = (subtotal_minor + fee_minor)) AND (credits > 0)))
);

CREATE TABLE public.otp_codes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    email text NOT NULL,
    token_hash text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    used_at timestamp with time zone,
    created_at timestamp with time zone
);

CREATE TABLE public.password_resets (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    token_hash text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    used_at timestamp with time zone,
    created_at timestamp with time zone
);

CREATE TABLE public.rag_embedding_models (
    id text NOT NULL,
    provider text NOT NULL,
    model_name text NOT NULL,
    dimension bigint NOT NULL,
    max_input_tokens bigint NOT NULL,
    dataset_name text NOT NULL,
    query_prefix text,
    passage_prefix text,
    pricing_per_1m_tokens_usd numeric NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE public.rag_index_attempt_errors (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    index_attempt_id uuid NOT NULL,
    rag_source_id uuid NOT NULL,
    document_id text,
    document_link text,
    entity_id text,
    failed_time_range_start timestamp with time zone,
    failed_time_range_end timestamp with time zone,
    failure_message text NOT NULL,
    is_resolved boolean DEFAULT false NOT NULL,
    error_type text,
    time_created timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);

CREATE TABLE public.rag_index_attempts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    rag_source_id uuid NOT NULL,
    embedding_model_id text,
    from_beginning boolean DEFAULT false NOT NULL,
    status text NOT NULL,
    new_docs_indexed bigint DEFAULT 0,
    total_docs_indexed bigint DEFAULT 0,
    docs_removed_from_index bigint DEFAULT 0,
    docs_estimated integer,
    error_msg text,
    full_exception_trace text,
    poll_range_start timestamp with time zone,
    poll_range_end timestamp with time zone,
    checkpoint_pointer text,
    celery_task_id text,
    cancellation_requested boolean DEFAULT false NOT NULL,
    total_batches bigint,
    completed_batches bigint DEFAULT 0 NOT NULL,
    total_failures_batch_level bigint DEFAULT 0 NOT NULL,
    total_chunks bigint DEFAULT 0 NOT NULL,
    last_progress_time timestamp with time zone,
    last_batches_completed_count bigint DEFAULT 0 NOT NULL,
    heartbeat_counter bigint DEFAULT 0 NOT NULL,
    last_heartbeat_value bigint DEFAULT 0 NOT NULL,
    last_heartbeat_time timestamp with time zone,
    time_created timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    time_started timestamp with time zone,
    time_updated timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);

CREATE TABLE public.rag_search_settings (
    org_id uuid NOT NULL,
    embedding_model_id character varying(128) NOT NULL,
    embedding_dim bigint NOT NULL,
    "normalize" boolean DEFAULT true NOT NULL,
    query_prefix text,
    passage_prefix text,
    embedding_precision character varying(16) DEFAULT 'float'::character varying NOT NULL,
    reduced_dimension integer,
    multipass_indexing boolean DEFAULT true NOT NULL,
    reranker_model_id character varying(128),
    hybrid_alpha double precision DEFAULT 0.7 NOT NULL,
    index_name character varying(256) NOT NULL,
    enable_contextual_rag boolean DEFAULT false NOT NULL,
    contextual_ragllm_name character varying(128),
    contextual_ragllm_provider character varying(64),
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE public.rag_sources (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    kind character varying(32) NOT NULL,
    name text NOT NULL,
    status character varying(32) NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    config jsonb DEFAULT '{}'::jsonb NOT NULL,
    connection_id uuid,
    indexing_start timestamp with time zone,
    last_successful_index_time timestamp with time zone,
    last_pruned timestamp with time zone,
    refresh_freq_seconds integer,
    prune_freq_seconds integer,
    total_docs_indexed bigint DEFAULT 0 NOT NULL,
    in_repeated_error_state boolean DEFAULT false NOT NULL,
    deletion_failure_message text,
    creator_id uuid,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE public.rag_sync_records (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    entity_id uuid NOT NULL,
    sync_type text NOT NULL,
    sync_status text NOT NULL,
    num_docs_synced bigint DEFAULT 0 NOT NULL,
    sync_start_time timestamp with time zone NOT NULL,
    sync_end_time timestamp with time zone
);

CREATE TABLE public.rag_sync_states (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    rag_source_id uuid NOT NULL,
    status character varying(32) NOT NULL,
    in_repeated_error_state boolean DEFAULT false NOT NULL,
    last_successful_index_time timestamp with time zone,
    last_pruned timestamp with time zone,
    last_time_hierarchy_fetch timestamp with time zone,
    total_docs_indexed bigint DEFAULT 0 NOT NULL,
    indexing_trigger character varying(16),
    processing_mode character varying(16) DEFAULT 'REGULAR'::character varying NOT NULL,
    deletion_failure_message text,
    creator_id uuid,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE public.refresh_tokens (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    token_hash text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone,
    replaced_at timestamp with time zone,
    replaced_by_access_token text,
    replaced_by_refresh_token text,
    created_at timestamp with time zone
);

CREATE TABLE public.sandbox_templates (
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

CREATE TABLE public.sandbox_warm_slots (
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
    updated_at timestamp with time zone,
    image_kind text DEFAULT 'default'::text NOT NULL,
    sandbox_size text DEFAULT 'small'::text NOT NULL,
    cpu integer DEFAULT 0 NOT NULL,
    memory integer DEFAULT 0 NOT NULL,
    disk integer DEFAULT 0 NOT NULL
);

CREATE TABLE public.sandboxes (
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
    updated_at timestamp with time zone,
    last_preview_at timestamp with time zone,
    exposed_ports integer[] DEFAULT '{3000,5173,8000,8080}'::integer[] NOT NULL,
    last_app_activity_at timestamp with time zone,
    last_gateway_activity_at timestamp with time zone
);

CREATE TABLE public.session_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    session_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    sandbox_id uuid,
    runtime_session_id text,
    event_id text,
    event_type text NOT NULL,
    actor_user_id uuid,
    source text DEFAULT 'web'::text NOT NULL,
    sequence_number bigint,
    payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    event_at timestamp with time zone NOT NULL,
    retained_at timestamp with time zone,
    created_at timestamp with time zone,
    runtime_seq bigint,
    runtime_event_id text,
    turn_id text,
    span_id text,
    durability text
);

CREATE TABLE public.session_message_queue (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    session_id uuid NOT NULL,
    session_event_id uuid,
    sequence_number bigint NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    attempt_count integer DEFAULT 0 NOT NULL,
    leased_by text,
    leased_until timestamp with time zone,
    delivered_at timestamp with time zone,
    last_error text DEFAULT ''::text NOT NULL,
    runtime_stream_id text DEFAULT ''::text NOT NULL,
    runtime_stream_url text DEFAULT ''::text NOT NULL,
    runtime_trace_id text DEFAULT ''::text NOT NULL,
    runtime_turn_id text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    actor_user_id uuid,
    message_text text DEFAULT ''::text NOT NULL,
    message_payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    model text DEFAULT ''::text NOT NULL,
    reasoning_effort text DEFAULT ''::text NOT NULL
);

CREATE TABLE public.session_participants (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    session_id uuid NOT NULL,
    user_id uuid NOT NULL,
    role text DEFAULT 'collaborator'::text NOT NULL,
    invited_by uuid,
    joined_at timestamp with time zone,
    last_seen_at timestamp with time zone,
    created_at timestamp with time zone
);

CREATE TABLE public.session_reflection_states (
    session_id uuid NOT NULL,
    org_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    last_reflected_event_id uuid,
    last_reflected_event_at timestamp with time zone,
    last_reflected_runtime_seq bigint,
    status text DEFAULT 'idle'::text NOT NULL,
    locked_until timestamp with time zone,
    last_error text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT session_reflection_states_status_check CHECK ((status = ANY (ARRAY['idle'::text, 'running'::text, 'failed'::text])))
);

CREATE TABLE public.sessions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    team_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    sandbox_id uuid,
    created_by uuid,
    model text,
    reasoning_effort text DEFAULT 'high'::text NOT NULL,
    source text DEFAULT 'web'::text NOT NULL,
    source_id uuid,
    source_resource_key text,
    name text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    agent_turn_status text DEFAULT 'idle'::text NOT NULL,
    agent_turn_id text DEFAULT ''::text NOT NULL,
    agent_stream_id text DEFAULT ''::text NOT NULL,
    agent_turn_started_at timestamp with time zone,
    integration_scopes jsonb,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    ended_at timestamp with time zone,
    session_name_auto_generated_at timestamp with time zone,
    agent_turn_last_outcome text DEFAULT ''::text NOT NULL,
    image_model text DEFAULT ''::text NOT NULL,
    vector_image_model text DEFAULT ''::text NOT NULL,
    runtime_mcp_actor_user_id uuid,
    runtime_mcp_config_version bigint DEFAULT 0 NOT NULL
);

CREATE TABLE public.sheet_fields (
    id text NOT NULL,
    page_id uuid NOT NULL,
    org_id uuid NOT NULL,
    name text NOT NULL,
    type text NOT NULL,
    options jsonb DEFAULT '{}'::jsonb NOT NULL,
    "position" double precision NOT NULL,
    archived_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.sheet_import_jobs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    page_id uuid NOT NULL,
    object_key text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    total_rows bigint DEFAULT 0 NOT NULL,
    processed_rows bigint DEFAULT 0 NOT NULL,
    error text DEFAULT ''::text NOT NULL,
    options jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_by_agent_id uuid,
    created_by_user_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT sheet_import_jobs_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'running'::text, 'completed'::text, 'failed'::text])))
);

CREATE TABLE public.sheet_operations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    page_id uuid NOT NULL,
    type text NOT NULL,
    row_count integer DEFAULT 0 NOT NULL,
    inverse jsonb DEFAULT '{}'::jsonb NOT NULL,
    actor_agent_id uuid,
    actor_user_id uuid,
    source_session_id uuid,
    reverted_at timestamp with time zone,
    reverted_by_agent_id uuid,
    reverted_by_user_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT sheet_operations_type_check CHECK ((type = ANY (ARRAY['rows_insert'::text, 'rows_update'::text, 'rows_delete'::text, 'csv_import'::text, 'field_change'::text])))
);

CREATE TABLE public.sheet_pages (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    sheet_id uuid NOT NULL,
    org_id uuid NOT NULL,
    name text NOT NULL,
    "position" double precision NOT NULL,
    display_field_id text,
    archived_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.sheet_rows (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    page_id uuid NOT NULL,
    org_id uuid NOT NULL,
    data jsonb DEFAULT '{}'::jsonb NOT NULL,
    "position" double precision NOT NULL,
    import_job_id uuid,
    created_by_agent_id uuid,
    created_by_user_id uuid,
    archived_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.sheet_views (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    page_id uuid NOT NULL,
    org_id uuid NOT NULL,
    name text NOT NULL,
    type text DEFAULT 'grid'::text NOT NULL,
    config jsonb DEFAULT '{}'::jsonb NOT NULL,
    "position" double precision NOT NULL,
    archived_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.sheets (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    slug text NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    icon text DEFAULT ''::text NOT NULL,
    created_by_agent_id uuid,
    created_by_user_id uuid,
    source_session_id uuid,
    archived_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    team_id uuid NOT NULL
);

CREATE TABLE public.skills (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    team_id uuid,
    publisher_id uuid,
    slug text NOT NULL,
    name text NOT NULL,
    description text,
    category character varying(64) DEFAULT ''::character varying NOT NULL,
    source_type text NOT NULL,
    repo_url text,
    repo_subpath text,
    repo_ref text DEFAULT 'main'::text NOT NULL,
    bundle jsonb DEFAULT '{}'::jsonb NOT NULL,
    hydrated_commit_sha text,
    hydrated_at timestamp with time zone,
    hydration_error text,
    tags text[] DEFAULT '{}'::text[],
    hidden boolean DEFAULT false NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    human_description text
);

CREATE TABLE public.slack_thread_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    connection_id uuid NOT NULL,
    team_id uuid,
    agent_id uuid,
    session_id uuid,
    session_event_id uuid,
    session_message_queue_id uuid,
    slack_team_id text DEFAULT ''::text NOT NULL,
    slack_channel_id text NOT NULL,
    thread_ts text NOT NULL,
    message_ts text NOT NULL,
    message_at timestamp with time zone NOT NULL,
    event_id text DEFAULT ''::text NOT NULL,
    event_type text NOT NULL,
    direction text NOT NULL,
    sender_id text DEFAULT ''::text NOT NULL,
    text text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'received'::text NOT NULL,
    ignore_reason text DEFAULT ''::text NOT NULL,
    slack_reply_ts text DEFAULT ''::text NOT NULL,
    runtime_stream_id text DEFAULT ''::text NOT NULL,
    runtime_turn_id text DEFAULT ''::text NOT NULL,
    error text DEFAULT ''::text NOT NULL,
    raw jsonb DEFAULT '{}'::jsonb NOT NULL,
    received_at timestamp with time zone DEFAULT now() NOT NULL,
    status_set_at timestamp with time zone,
    enqueued_at timestamp with time zone,
    job_started_at timestamp with time zone,
    route_resolved_at timestamp with time zone,
    session_resolved_at timestamp with time zone,
    runtime_posted_at timestamp with time zone,
    final_received_at timestamp with time zone,
    slack_reply_sent_at timestamp with time zone,
    completed_at timestamp with time zone,
    failed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    trigger_id uuid
);

CREATE TABLE public.team_connection_grants (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    team_id uuid NOT NULL,
    connection_id uuid,
    database_connection_id uuid,
    granted_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT team_connection_grants_one_connection_check CHECK ((((connection_id IS NOT NULL) AND (database_connection_id IS NULL)) OR ((connection_id IS NULL) AND (database_connection_id IS NOT NULL))))
);

CREATE TABLE public.team_env_vars (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    team_id uuid NOT NULL,
    name text NOT NULL,
    encrypted_value bytea NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.team_mcp_servers (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    team_id uuid NOT NULL,
    mcp_server_id uuid NOT NULL,
    granted_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.team_members (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    team_id uuid NOT NULL,
    user_id uuid NOT NULL,
    role text DEFAULT 'member'::text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deactivated_at timestamp with time zone
);

CREATE TABLE public.team_rag_sources (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    team_id uuid NOT NULL,
    rag_source_id uuid NOT NULL,
    granted_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.team_skill_grants (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    team_id uuid NOT NULL,
    skill_id uuid NOT NULL,
    granted_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.teams (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    created_by uuid,
    archived_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE public.tokens (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    credential_id uuid NOT NULL,
    jti text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    remaining bigint,
    refill_amount bigint,
    refill_interval text,
    last_refill_at timestamp with time zone,
    scopes jsonb,
    meta jsonb DEFAULT '{}'::jsonb,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone
);

CREATE TABLE public.tool_usages (
    id text NOT NULL,
    org_id uuid NOT NULL,
    agent_id text NOT NULL,
    token_jti text NOT NULL,
    tool_name text NOT NULL,
    input text,
    pages_returned bigint DEFAULT 0,
    status text NOT NULL,
    error_message text,
    total_ms bigint,
    credits_used bigint DEFAULT 0,
    ip_address inet,
    created_at timestamp with time zone NOT NULL
);

CREATE TABLE public.usage (
    id bigint NOT NULL,
    org_id uuid NOT NULL,
    credential_id uuid NOT NULL,
    request_count bigint DEFAULT 0 NOT NULL,
    period_start timestamp with time zone NOT NULL,
    period_end timestamp with time zone NOT NULL,
    created_at timestamp with time zone
);

CREATE SEQUENCE public.usage_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.usage_id_seq OWNED BY public.usage.id;

CREATE TABLE public.user_agent_mcp_servers (
    org_id uuid NOT NULL,
    user_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    mcp_server_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE public.users (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    email text NOT NULL,
    password_hash text,
    name text,
    avatar_url text,
    email_confirmed_at timestamp with time zone,
    banned_at timestamp with time zone,
    ban_reason text,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY public.audit_log ALTER COLUMN id SET DEFAULT nextval('public.audit_log_id_seq'::regclass);

ALTER TABLE ONLY public.usage ALTER COLUMN id SET DEFAULT nextval('public.usage_id_seq'::regclass);

ALTER TABLE ONLY public.agent_assets
    ADD CONSTRAINT agent_assets_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.agent_catalog
    ADD CONSTRAINT agent_catalog_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.agent_directives
    ADD CONSTRAINT agent_directives_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.agent_mcp_servers
    ADD CONSTRAINT agent_mcp_servers_pkey PRIMARY KEY (agent_id, mcp_server_id);

ALTER TABLE ONLY public.agent_memories
    ADD CONSTRAINT agent_memories_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.agent_observations
    ADD CONSTRAINT agent_observations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.agent_schedule_runs
    ADD CONSTRAINT agent_schedule_runs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.agent_schedules
    ADD CONSTRAINT agent_schedules_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.agent_trigger_deliveries
    ADD CONSTRAINT agent_trigger_deliveries_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.agent_triggers
    ADD CONSTRAINT agent_triggers_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT agents_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.agent_memory_digests
    ADD CONSTRAINT agent_memory_digests_pkey PRIMARY KEY (agent_id);

ALTER TABLE ONLY public.agent_email_threads
    ADD CONSTRAINT agent_email_threads_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.agent_email_messages
    ADD CONSTRAINT agent_email_messages_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.agent_email_webhook_receipts
    ADD CONSTRAINT agent_email_webhook_receipts_pkey PRIMARY KEY (svix_id);

ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT api_keys_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.app_versions
    ADD CONSTRAINT app_versions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.apps
    ADD CONSTRAINT apps_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.billing_payment_methods
    ADD CONSTRAINT billing_payment_methods_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.audit_log
    ADD CONSTRAINT audit_log_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.brand_assets
    ADD CONSTRAINT brand_assets_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.brands
    ADD CONSTRAINT brands_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.canvas_artifact_files
    ADD CONSTRAINT canvas_artifact_files_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.canvas_artifacts
    ADD CONSTRAINT canvas_artifacts_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.canvas_projects
    ADD CONSTRAINT canvas_projects_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.connections
    ADD CONSTRAINT connections_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.credit_purchases
    ADD CONSTRAINT credit_purchases_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.credentials
    ADD CONSTRAINT credentials_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.credit_ledger_entries
    ADD CONSTRAINT credit_ledger_entries_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.database_connections
    ADD CONSTRAINT database_connections_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.drive_assets
    ADD CONSTRAINT drive_assets_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.email_verifications
    ADD CONSTRAINT email_verifications_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.generations
    ADD CONSTRAINT generations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.github_pull_request_sessions
    ADD CONSTRAINT github_pull_request_sessions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.integrations
    ADD CONSTRAINT integrations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.mcp_authorizations
    ADD CONSTRAINT mcp_authorizations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.mcp_oauth_states
    ADD CONSTRAINT mcp_oauth_states_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.mcp_oauth_states
    ADD CONSTRAINT mcp_oauth_states_state_hash_key UNIQUE (state_hash);

ALTER TABLE ONLY public.mcp_servers
    ADD CONSTRAINT mcp_servers_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.memory_suppressions
    ADD CONSTRAINT memory_suppressions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.memory_suppressions
    ADD CONSTRAINT memory_suppressions_unique UNIQUE (org_id, agent_id, content_fingerprint);

ALTER TABLE ONLY public.oauth_accounts
    ADD CONSTRAINT oauth_accounts_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.oauth_exchange_tokens
    ADD CONSTRAINT oauth_exchange_tokens_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.org_invite_teams
    ADD CONSTRAINT org_invite_teams_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.org_invites
    ADD CONSTRAINT org_invites_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.org_memberships
    ADD CONSTRAINT org_memberships_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.orgs
    ADD CONSTRAINT orgs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.otp_codes
    ADD CONSTRAINT otp_codes_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.password_resets
    ADD CONSTRAINT password_resets_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.rag_embedding_models
    ADD CONSTRAINT rag_embedding_models_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.rag_index_attempt_errors
    ADD CONSTRAINT rag_index_attempt_errors_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.rag_index_attempts
    ADD CONSTRAINT rag_index_attempts_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.rag_search_settings
    ADD CONSTRAINT rag_search_settings_pkey PRIMARY KEY (org_id);

ALTER TABLE ONLY public.rag_sources
    ADD CONSTRAINT rag_sources_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.rag_sync_records
    ADD CONSTRAINT rag_sync_records_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.rag_sync_states
    ADD CONSTRAINT rag_sync_states_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.refresh_tokens
    ADD CONSTRAINT refresh_tokens_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.sandbox_templates
    ADD CONSTRAINT sandbox_templates_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.sandbox_warm_slots
    ADD CONSTRAINT sandbox_warm_slots_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.sandboxes
    ADD CONSTRAINT sandboxes_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.session_events
    ADD CONSTRAINT session_events_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.session_message_queue
    ADD CONSTRAINT session_message_queue_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.session_participants
    ADD CONSTRAINT session_participants_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.session_reflection_states
    ADD CONSTRAINT session_reflection_states_pkey PRIMARY KEY (session_id);

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT sessions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.sheet_fields
    ADD CONSTRAINT sheet_fields_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.sheet_import_jobs
    ADD CONSTRAINT sheet_import_jobs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.sheet_operations
    ADD CONSTRAINT sheet_operations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.sheet_pages
    ADD CONSTRAINT sheet_pages_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.sheet_rows
    ADD CONSTRAINT sheet_rows_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.sheet_views
    ADD CONSTRAINT sheet_views_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.sheets
    ADD CONSTRAINT sheets_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.skills
    ADD CONSTRAINT skills_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.slack_thread_events
    ADD CONSTRAINT slack_thread_events_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.team_connection_grants
    ADD CONSTRAINT team_connection_grants_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.team_external_resource_routes
    ADD CONSTRAINT team_external_resource_routes_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.team_env_vars
    ADD CONSTRAINT team_env_vars_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.team_mcp_servers
    ADD CONSTRAINT team_mcp_servers_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.team_mcp_servers
    ADD CONSTRAINT team_mcp_servers_unique UNIQUE (team_id, mcp_server_id);

ALTER TABLE ONLY public.team_members
    ADD CONSTRAINT team_members_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.team_rag_sources
    ADD CONSTRAINT team_rag_sources_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.team_rag_sources
    ADD CONSTRAINT team_rag_sources_team_source_unique UNIQUE (team_id, rag_source_id);

ALTER TABLE ONLY public.team_skill_grants
    ADD CONSTRAINT team_skill_grants_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.team_skill_grants
    ADD CONSTRAINT team_skill_grants_team_skill_unique UNIQUE (team_id, skill_id);

ALTER TABLE ONLY public.teams
    ADD CONSTRAINT teams_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.tokens
    ADD CONSTRAINT tokens_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.tool_usages
    ADD CONSTRAINT tool_usages_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.usage
    ADD CONSTRAINT usage_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.user_agent_mcp_servers
    ADD CONSTRAINT user_agent_mcp_servers_pkey PRIMARY KEY (user_id, agent_id, mcp_server_id);

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX idx_agent_assets_key ON public.agent_assets USING btree (key);

CREATE INDEX idx_agent_assets_org_id ON public.agent_assets USING btree (org_id);

CREATE INDEX idx_agent_catalog_default ON public.agent_catalog USING btree (is_default);

CREATE UNIQUE INDEX idx_agent_catalog_slug ON public.agent_catalog USING btree (slug);

CREATE INDEX idx_agent_catalog_status ON public.agent_catalog USING btree (status);

CREATE INDEX idx_agent_directives_org_agent ON public.agent_directives USING btree (org_id, agent_id) WHERE active;

CREATE INDEX idx_agent_mcp_servers_org ON public.agent_mcp_servers USING btree (org_id);

CREATE INDEX idx_agent_mcp_servers_server ON public.agent_mcp_servers USING btree (mcp_server_id);

CREATE INDEX idx_agent_memories_embedding_hnsw ON public.agent_memories USING hnsw (embedding public.vector_cosine_ops) WHERE ((archived_at IS NULL) AND (embedding_status = 'ready'::text));

CREATE INDEX idx_agent_memories_embedding_status ON public.agent_memories USING btree (embedding_status, updated_at) WHERE (archived_at IS NULL);

CREATE UNIQUE INDEX idx_agent_memories_fingerprint ON public.agent_memories USING btree (memory_fingerprint) WHERE ((archived_at IS NULL) AND (memory_fingerprint <> ''::text));

CREATE INDEX idx_agent_memories_org_agent ON public.agent_memories USING btree (org_id, agent_id, created_at DESC) WHERE (archived_at IS NULL);

CREATE INDEX idx_agent_memories_tags ON public.agent_memories USING gin (tags) WHERE (archived_at IS NULL);

CREATE INDEX idx_agent_memories_unconsolidated ON public.agent_memories USING btree (org_id, agent_id, created_at) WHERE ((archived_at IS NULL) AND (consolidated_at IS NULL));

CREATE INDEX idx_agent_observations_embedding_hnsw ON public.agent_observations USING hnsw (embedding public.vector_cosine_ops) WHERE ((archived_at IS NULL) AND (embedding_status = 'ready'::text));

CREATE INDEX idx_agent_observations_expires ON public.agent_observations USING btree (expires_at) WHERE ((archived_at IS NULL) AND (expires_at IS NOT NULL));

CREATE INDEX idx_agent_observations_org_agent ON public.agent_observations USING btree (org_id, agent_id) WHERE (archived_at IS NULL);

CREATE INDEX idx_agent_org_id ON public.agents USING btree (org_id);

CREATE UNIQUE INDEX idx_agent_schedule_agent_runtime ON public.agent_schedules USING btree (agent_id, runtime_job_id);

CREATE UNIQUE INDEX idx_agent_schedule_run_key ON public.agent_schedule_runs USING btree (schedule_id, run_key);

CREATE INDEX idx_agent_schedule_runs_agent_id ON public.agent_schedule_runs USING btree (agent_id);

CREATE INDEX idx_agent_schedule_runs_lease ON public.agent_schedule_runs USING btree (leased_until) WHERE (lease_owner <> ''::text);

CREATE INDEX idx_agent_schedule_runs_org_id ON public.agent_schedule_runs USING btree (org_id);

CREATE INDEX idx_agent_schedule_runs_runtime_job_id ON public.agent_schedule_runs USING btree (runtime_job_id);

CREATE INDEX idx_agent_schedule_runs_sandbox_id ON public.agent_schedule_runs USING btree (sandbox_id);

CREATE INDEX idx_agent_schedule_runs_scheduled_at ON public.agent_schedule_runs USING btree (scheduled_at);

CREATE INDEX idx_agent_schedule_runs_session_id ON public.agent_schedule_runs USING btree (session_id);

CREATE INDEX idx_agent_schedule_runs_status ON public.agent_schedule_runs USING btree (status);

CREATE UNIQUE INDEX idx_agent_schedules_agent_source_active ON public.agent_schedules USING btree (agent_id, source_slug) WHERE ((source_slug <> ''::text) AND (cancelled_at IS NULL) AND ((status)::text <> 'cancelled'::text));

CREATE INDEX idx_agent_schedules_cancelled_at ON public.agent_schedules USING btree (cancelled_at);

CREATE INDEX idx_agent_schedules_connection_id ON public.agent_schedules USING btree (connection_id);

CREATE INDEX idx_agent_schedules_created_by_user_id ON public.agent_schedules USING btree (created_by_user_id);

CREATE INDEX idx_agent_schedules_due ON public.agent_schedules USING btree (status, next_run_at) WHERE (cancelled_at IS NULL);

CREATE INDEX idx_agent_schedules_is_system ON public.agent_schedules USING btree (is_system);

CREATE INDEX idx_agent_schedules_lease ON public.agent_schedules USING btree (leased_until) WHERE (lease_owner <> ''::text);

CREATE INDEX idx_agent_schedules_next_run_at ON public.agent_schedules USING btree (next_run_at);

CREATE INDEX idx_agent_schedules_org_id ON public.agent_schedules USING btree (org_id);

CREATE INDEX idx_agent_schedules_provider ON public.agent_schedules USING btree (provider);

CREATE INDEX idx_agent_schedules_sandbox_id ON public.agent_schedules USING btree (sandbox_id);

CREATE INDEX idx_agent_schedules_status ON public.agent_schedules USING btree (status);

CREATE INDEX idx_agent_trigger_deliveries_connection_id ON public.agent_trigger_deliveries USING btree (connection_id);

CREATE INDEX idx_agent_trigger_deliveries_delivery_id ON public.agent_trigger_deliveries USING btree (delivery_id);

CREATE INDEX idx_agent_trigger_deliveries_event_key ON public.agent_trigger_deliveries USING btree (event_key);

CREATE INDEX idx_agent_trigger_deliveries_resource_key ON public.agent_trigger_deliveries USING btree (resource_key);

CREATE INDEX idx_agent_trigger_deliveries_runtime_session_id ON public.agent_trigger_deliveries USING btree (runtime_session_id);

CREATE INDEX idx_agent_trigger_deliveries_session_id ON public.agent_trigger_deliveries USING btree (session_id);

CREATE UNIQUE INDEX idx_agent_trigger_deliveries_trigger_delivery ON public.agent_trigger_deliveries USING btree (trigger_id, delivery_id) WHERE (delivery_id <> ''::text);

CREATE INDEX idx_agent_trigger_deliveries_trigger_id ON public.agent_trigger_deliveries USING btree (trigger_id);

CREATE INDEX idx_agent_triggers_agent_id ON public.agent_triggers USING btree (agent_id);

CREATE UNIQUE INDEX idx_agent_triggers_agent_source_active ON public.agent_triggers USING btree (agent_id, source_slug) WHERE ((source_slug <> ''::text) AND (enabled = true));

CREATE INDEX idx_agent_triggers_connection_id ON public.agent_triggers USING btree (connection_id);

CREATE UNIQUE INDEX idx_agent_triggers_enabled_key_value ON public.agent_triggers USING btree (org_id, connection_id, resource_type, resource_key, trigger_key, trigger_value) WHERE ((enabled = true) AND (trigger_key <> ''::text) AND (trigger_value <> ''::text));

CREATE INDEX idx_agent_triggers_org_id ON public.agent_triggers USING btree (org_id);

CREATE INDEX idx_agent_triggers_trigger_key ON public.agent_triggers USING btree (trigger_key);

CREATE INDEX idx_agent_triggers_trigger_type ON public.agent_triggers USING btree (trigger_type);

CREATE INDEX idx_agents_agent_catalog_id ON public.agents USING btree (agent_catalog_id);

CREATE UNIQUE INDEX idx_agents_id_org_id ON public.agents USING btree (id, org_id);

CREATE INDEX idx_agents_is_default ON public.agents USING btree (is_default);

CREATE UNIQUE INDEX idx_agents_org_agent_catalog_team_active ON public.agents USING btree (org_id, agent_catalog_id, COALESCE(team_id, '00000000-0000-0000-0000-000000000000'::uuid)) WHERE ((agent_catalog_id IS NOT NULL) AND (status <> 'archived'::text));

CREATE INDEX idx_agents_parent_agent_id ON public.agents USING btree (parent_agent_id);

CREATE UNIQUE INDEX idx_agents_parent_name ON public.agents USING btree (parent_agent_id, name) WHERE (parent_agent_id IS NOT NULL);

CREATE INDEX idx_agents_team_id ON public.agents USING btree (team_id);

CREATE UNIQUE INDEX idx_agents_default_team_active ON public.agents USING btree (team_id) WHERE ((is_default = true) AND (type = 'agent'::text) AND (status <> 'archived'::text));

CREATE UNIQUE INDEX idx_agents_email_inbox_local_part ON public.agents USING btree (email_inbox_local_part) WHERE (email_inbox_local_part <> ''::text);

CREATE INDEX idx_agent_email_threads_org_agent ON public.agent_email_threads USING btree (org_id, agent_id);

CREATE INDEX idx_agent_email_threads_session ON public.agent_email_threads USING btree (session_id);

CREATE UNIQUE INDEX idx_agent_email_threads_reply_token ON public.agent_email_threads USING btree (reply_token);

CREATE UNIQUE INDEX idx_agent_email_messages_resend_id ON public.agent_email_messages USING btree (resend_email_id) WHERE (resend_email_id <> ''::text);

CREATE INDEX idx_agent_email_messages_agent_message_id ON public.agent_email_messages USING btree (agent_id, message_id) WHERE (message_id <> ''::text);

CREATE INDEX idx_agent_email_messages_thread_provider_at ON public.agent_email_messages USING btree (thread_id, provider_at);

CREATE INDEX idx_agent_email_webhook_receipts_resend_email_id ON public.agent_email_webhook_receipts USING btree (resend_email_id);

CREATE INDEX idx_api_keys_created_by ON public.api_keys USING btree (created_by);

CREATE UNIQUE INDEX idx_api_keys_key_hash ON public.api_keys USING btree (key_hash);

CREATE INDEX idx_api_keys_org_id ON public.api_keys USING btree (org_id);

CREATE INDEX idx_app_versions_app_created_active ON public.app_versions USING btree (app_id, created_at DESC) WHERE (archived_at IS NULL);

CREATE INDEX idx_app_versions_org ON public.app_versions USING btree (org_id);

CREATE INDEX idx_apps_team_updated_active ON public.apps USING btree (team_id, updated_at DESC) WHERE (archived_at IS NULL);

CREATE UNIQUE INDEX idx_apps_org_slug_active ON public.apps USING btree (org_id, slug) WHERE (archived_at IS NULL);

CREATE INDEX idx_apps_sheet_active ON public.apps USING btree (sheet_id) WHERE (archived_at IS NULL);

CREATE INDEX idx_billing_payment_methods_org_id ON public.billing_payment_methods USING btree (org_id);

CREATE INDEX idx_billing_payment_methods_user_id ON public.billing_payment_methods USING btree (user_id);

CREATE UNIQUE INDEX idx_billing_payment_methods_user_signature ON public.billing_payment_methods USING btree (org_id, user_id, provider, provider_signature);

CREATE INDEX idx_audit_credential ON public.audit_log USING btree (credential_id);

CREATE INDEX idx_audit_org_created ON public.audit_log USING btree (org_id, created_at);

CREATE UNIQUE INDEX idx_brand_assets_key ON public.brand_assets USING btree (key);

CREATE INDEX idx_brand_assets_kind ON public.brand_assets USING btree (kind);

CREATE INDEX idx_brand_assets_org_brand ON public.brand_assets USING btree (org_id, brand_id);

CREATE INDEX idx_brands_archived_at ON public.brands USING btree (archived_at);

CREATE INDEX idx_brands_org_created ON public.brands USING btree (org_id, created_at DESC) WHERE (archived_at IS NULL);

CREATE UNIQUE INDEX idx_brands_org_default_active ON public.brands USING btree (org_id) WHERE (is_default AND (archived_at IS NULL));

CREATE UNIQUE INDEX idx_brands_org_slug_active ON public.brands USING btree (org_id, slug) WHERE (archived_at IS NULL);

CREATE INDEX idx_canvas_artifact_files_archived_at ON public.canvas_artifact_files USING btree (archived_at);

CREATE UNIQUE INDEX idx_canvas_artifact_files_artifact_path_active ON public.canvas_artifact_files USING btree (canvas_artifact_id, path) WHERE (archived_at IS NULL);

CREATE INDEX idx_canvas_artifact_files_object_key ON public.canvas_artifact_files USING btree (object_key);

CREATE INDEX idx_canvas_artifact_files_org ON public.canvas_artifact_files USING btree (org_id);

CREATE INDEX idx_canvas_artifacts_archived_at ON public.canvas_artifacts USING btree (archived_at);

CREATE INDEX idx_canvas_artifacts_org_project ON public.canvas_artifacts USING btree (org_id, canvas_project_id) WHERE (archived_at IS NULL);

CREATE UNIQUE INDEX idx_canvas_artifacts_project_slug_active ON public.canvas_artifacts USING btree (canvas_project_id, slug) WHERE (archived_at IS NULL);

CREATE INDEX idx_canvas_artifacts_source_session ON public.canvas_artifacts USING btree (source_session_id) WHERE (source_session_id IS NOT NULL);

CREATE INDEX idx_canvas_projects_archived_at ON public.canvas_projects USING btree (archived_at);

CREATE INDEX idx_canvas_projects_org_id ON public.canvas_projects USING btree (org_id);

CREATE UNIQUE INDEX idx_canvas_projects_org_slug_active ON public.canvas_projects USING btree (org_id, slug) WHERE ((archived_at IS NULL) AND (slug <> ''::text));

CREATE INDEX idx_canvas_projects_org_updated_active ON public.canvas_projects USING btree (org_id, updated_at DESC) WHERE (archived_at IS NULL);

CREATE UNIQUE INDEX idx_connections_active_nango_id ON public.connections USING btree (integration_id, nango_connection_id) WHERE (revoked_at IS NULL);

CREATE UNIQUE INDEX idx_connections_active_org_slug ON public.connections USING btree (org_id, slug) WHERE (revoked_at IS NULL);

CREATE UNIQUE INDEX idx_connections_id_org_id ON public.connections USING btree (id, org_id);

CREATE INDEX idx_connections_integration_id ON public.connections USING btree (integration_id);

CREATE INDEX idx_connections_org_id ON public.connections USING btree (org_id);

CREATE INDEX idx_connections_user_id ON public.connections USING btree (user_id);

CREATE INDEX idx_credentials_org_id ON public.credentials USING btree (org_id);

CREATE UNIQUE INDEX idx_credit_ledger_entries_idem ON public.credit_ledger_entries USING btree (org_id, reason, ref_type, ref_id) WHERE ((ref_id)::text <> ''::text);

CREATE INDEX idx_credit_ledger_entries_org_id ON public.credit_ledger_entries USING btree (org_id);

CREATE INDEX idx_credit_ledger_entries_ref_id ON public.credit_ledger_entries USING btree (ref_id);

CREATE INDEX idx_credit_purchases_created_by_user_id ON public.credit_purchases USING btree (created_by_user_id);

CREATE UNIQUE INDEX idx_credit_purchases_org_id_idempotency_key ON public.credit_purchases USING btree (org_id, idempotency_key);

CREATE INDEX idx_credit_purchases_payment_method_id ON public.credit_purchases USING btree (payment_method_id);

CREATE INDEX idx_credit_purchases_org_id ON public.credit_purchases USING btree (org_id);

CREATE UNIQUE INDEX idx_credit_purchases_provider_reference ON public.credit_purchases USING btree (provider, provider_reference) WHERE ((provider_reference)::text <> ''::text);

CREATE INDEX idx_database_connections_active ON public.database_connections USING btree (org_id, provider) WHERE (revoked_at IS NULL);

CREATE UNIQUE INDEX idx_database_connections_active_org_slug ON public.database_connections USING btree (org_id, slug) WHERE (revoked_at IS NULL);

CREATE UNIQUE INDEX idx_database_connections_id_org_id ON public.database_connections USING btree (id, org_id);

CREATE INDEX idx_database_connections_org_provider ON public.database_connections USING btree (org_id, provider);

CREATE INDEX idx_drive_asset_agent ON public.drive_assets USING btree (agent_id);

CREATE INDEX idx_drive_asset_org ON public.drive_assets USING btree (org_id);

CREATE UNIQUE INDEX idx_drive_assets_s3_key ON public.drive_assets USING btree (s3_key);

CREATE UNIQUE INDEX idx_email_verifications_token_hash ON public.email_verifications USING btree (token_hash);

CREATE INDEX idx_email_verifications_user_id ON public.email_verifications USING btree (user_id);

CREATE INDEX idx_emp_asset_agent_created ON public.agent_assets USING btree (agent_id, created_at DESC);

CREATE INDEX idx_gen_billed_org_cost ON public.generations USING btree (org_id, cost) WHERE ((is_system = true) AND (billed_at IS NOT NULL) AND (billing_error = ''::text) AND (cost > (0)::numeric));

CREATE INDEX idx_gen_org_created ON public.generations USING btree (org_id, created_at);

CREATE INDEX idx_gen_org_credential ON public.generations USING btree (credential_id);

CREATE INDEX idx_gen_org_model ON public.generations USING btree (model);

CREATE INDEX idx_gen_org_provider ON public.generations USING btree (provider_id);

CREATE INDEX idx_gen_org_user ON public.generations USING btree (user_id);

CREATE INDEX idx_gen_session_id ON public.generations USING btree (session_id) WHERE (session_id IS NOT NULL);

CREATE INDEX idx_gen_unbilled_system_created ON public.generations USING btree (created_at) WHERE ((billed_at IS NULL) AND (is_system = true));

CREATE UNIQUE INDEX idx_github_pr_sessions_repo_number ON public.github_pull_request_sessions USING btree (org_id, repo, pr_number);

CREATE INDEX idx_github_pr_sessions_session ON public.github_pull_request_sessions USING btree (session_id);

CREATE INDEX idx_integrations_deleted_at ON public.integrations USING btree (deleted_at);

CREATE INDEX idx_integrations_provider ON public.integrations USING btree (provider);

CREATE UNIQUE INDEX idx_integrations_unique_key ON public.integrations USING btree (unique_key);

CREATE INDEX idx_mcp_authorizations_org ON public.mcp_authorizations USING btree (org_id);

CREATE INDEX idx_mcp_authorizations_server ON public.mcp_authorizations USING btree (mcp_server_id);

CREATE UNIQUE INDEX idx_mcp_authorizations_service ON public.mcp_authorizations USING btree (mcp_server_id) WHERE (principal_type = 'org_service'::text);

CREATE UNIQUE INDEX idx_mcp_authorizations_user ON public.mcp_authorizations USING btree (mcp_server_id, principal_user_id) WHERE (principal_type = 'user'::text);

CREATE INDEX idx_mcp_oauth_states_expires ON public.mcp_oauth_states USING btree (expires_at);

CREATE INDEX idx_mcp_oauth_states_org ON public.mcp_oauth_states USING btree (org_id);

CREATE INDEX idx_mcp_oauth_states_server ON public.mcp_oauth_states USING btree (mcp_server_id);

CREATE UNIQUE INDEX idx_mcp_servers_id_org ON public.mcp_servers USING btree (id, org_id);

CREATE INDEX idx_mcp_servers_org ON public.mcp_servers USING btree (org_id);

CREATE UNIQUE INDEX idx_mcp_servers_org_slug ON public.mcp_servers USING btree (org_id, slug) WHERE (scope = 'org'::text);

CREATE INDEX idx_mcp_servers_owner ON public.mcp_servers USING btree (owner_user_id);

CREATE UNIQUE INDEX idx_mcp_servers_personal_slug ON public.mcp_servers USING btree (org_id, owner_user_id, slug) WHERE (scope = 'personal'::text);

CREATE UNIQUE INDEX idx_membership_user_org ON public.org_memberships USING btree (user_id, org_id);

CREATE UNIQUE INDEX idx_oauth_exchange_tokens_token_hash ON public.oauth_exchange_tokens USING btree (token_hash);

CREATE INDEX idx_oauth_exchange_tokens_user_id ON public.oauth_exchange_tokens USING btree (user_id);

CREATE UNIQUE INDEX idx_oauth_provider_uid ON public.oauth_accounts USING btree (provider, provider_user_id);

CREATE UNIQUE INDEX idx_oauth_user_provider ON public.oauth_accounts USING btree (user_id, provider);

CREATE UNIQUE INDEX idx_org_invite_teams_invite_team ON public.org_invite_teams USING btree (org_invite_id, team_id);

CREATE INDEX idx_org_invite_teams_org_id ON public.org_invite_teams USING btree (org_id);

CREATE INDEX idx_org_invite_teams_team_id ON public.org_invite_teams USING btree (team_id);

CREATE INDEX idx_org_invites_email ON public.org_invites USING btree (email);

CREATE INDEX idx_org_invites_org_id ON public.org_invites USING btree (org_id);

CREATE UNIQUE INDEX idx_org_invites_token_hash ON public.org_invites USING btree (token_hash);

CREATE INDEX idx_otp_codes_email ON public.otp_codes USING btree (email);

CREATE UNIQUE INDEX idx_otp_codes_token_hash ON public.otp_codes USING btree (token_hash);

CREATE UNIQUE INDEX idx_password_resets_token_hash ON public.password_resets USING btree (token_hash);

CREATE INDEX idx_password_resets_user_id ON public.password_resets USING btree (user_id);

CREATE INDEX idx_rag_index_attempt_errors_index_attempt_id ON public.rag_index_attempt_errors USING btree (index_attempt_id);

CREATE INDEX idx_rag_index_attempt_errors_org_id ON public.rag_index_attempt_errors USING btree (org_id);

CREATE INDEX idx_rag_index_attempt_errors_rag_source_id ON public.rag_index_attempt_errors USING btree (rag_source_id);

CREATE INDEX idx_rag_index_attempts_org_id ON public.rag_index_attempts USING btree (org_id);

CREATE INDEX idx_rag_index_attempts_rag_source_id ON public.rag_index_attempts USING btree (rag_source_id);

CREATE INDEX idx_rag_index_attempts_status ON public.rag_index_attempts USING btree (status);

CREATE INDEX idx_rag_index_attempts_time_created ON public.rag_index_attempts USING btree (time_created);

CREATE INDEX idx_rag_search_settings_embedding_model_id ON public.rag_search_settings USING btree (embedding_model_id);

CREATE INDEX idx_rag_sync_records_org_id ON public.rag_sync_records USING btree (org_id);

CREATE INDEX idx_rag_sync_state_last_pruned ON public.rag_sync_states USING btree (last_pruned);

CREATE INDEX idx_rag_sync_states_org_id ON public.rag_sync_states USING btree (org_id);

CREATE UNIQUE INDEX idx_refresh_tokens_token_hash ON public.refresh_tokens USING btree (token_hash);

CREATE INDEX idx_refresh_tokens_user_id ON public.refresh_tokens USING btree (user_id);

CREATE INDEX idx_sandbox_templates_base_template_id ON public.sandbox_templates USING btree (base_template_id);

CREATE INDEX idx_sandbox_templates_org_id ON public.sandbox_templates USING btree (org_id);

CREATE UNIQUE INDEX idx_sandbox_templates_slug ON public.sandbox_templates USING btree (slug);

CREATE INDEX idx_sandbox_warm_slots_claimed_sandbox_id ON public.sandbox_warm_slots USING btree (claimed_sandbox_id);

CREATE INDEX idx_sandbox_warm_slots_pool_profile_status ON public.sandbox_warm_slots USING btree (provider_id, mode, image_kind, runtime_image, sandbox_size, cpu, memory, disk, status, created_at);

CREATE INDEX idx_sandbox_warm_slots_pool_status ON public.sandbox_warm_slots USING btree (provider_id, mode, status, created_at);

CREATE UNIQUE INDEX idx_sandbox_warm_slots_provider_external ON public.sandbox_warm_slots USING btree (provider_id, external_id);

CREATE INDEX idx_sandboxes_agent_id ON public.sandboxes USING btree (agent_id);

CREATE INDEX idx_sandboxes_last_preview_at ON public.sandboxes USING btree (last_preview_at);

CREATE INDEX idx_sandboxes_org_id ON public.sandboxes USING btree (org_id);

CREATE UNIQUE INDEX idx_session_events_idem ON public.session_events USING btree (session_id, event_id) WHERE (event_id IS NOT NULL);

CREATE UNIQUE INDEX idx_session_events_runtime_event_id ON public.session_events USING btree (session_id, runtime_event_id) WHERE ((runtime_event_id IS NOT NULL) AND (runtime_event_id <> ''::text));

CREATE UNIQUE INDEX idx_session_events_runtime_seq ON public.session_events USING btree (session_id, runtime_seq) WHERE (runtime_seq IS NOT NULL);

CREATE UNIQUE INDEX idx_session_events_runtime_sequence_number ON public.session_events USING btree (session_id, sequence_number) WHERE (runtime_seq IS NOT NULL);

CREATE INDEX idx_session_events_session ON public.session_events USING btree (session_id, event_at);

CREATE INDEX idx_session_events_session_sequence ON public.session_events USING btree (session_id, sequence_number) WHERE (sequence_number IS NOT NULL);

CREATE INDEX idx_session_message_queue_claim ON public.session_message_queue USING btree (session_id, status, sequence_number);

CREATE UNIQUE INDEX idx_session_message_queue_sequence ON public.session_message_queue USING btree (session_id, sequence_number);

CREATE UNIQUE INDEX idx_session_message_queue_session_event ON public.session_message_queue USING btree (session_id, session_event_id) WHERE (session_event_id IS NOT NULL);

CREATE UNIQUE INDEX idx_session_participants_session_user ON public.session_participants USING btree (session_id, user_id);

CREATE INDEX idx_session_participants_user_id ON public.session_participants USING btree (user_id);

CREATE INDEX idx_session_reflection_states_scan ON public.session_reflection_states USING btree (status, locked_until, updated_at);

CREATE INDEX idx_sessions_agent ON public.sessions USING btree (org_id, agent_id, created_at DESC);

CREATE INDEX idx_sessions_agent_turn_status ON public.sessions USING btree (agent_turn_status) WHERE (agent_turn_status <> 'idle'::text);

CREATE INDEX idx_sessions_team ON public.sessions USING btree (team_id, created_at DESC);

CREATE INDEX idx_sessions_runtime_mcp_actor_user_id ON public.sessions USING btree (runtime_mcp_actor_user_id);

CREATE INDEX idx_sessions_sandbox_id ON public.sessions USING btree (sandbox_id);

CREATE INDEX idx_sheet_fields_org ON public.sheet_fields USING btree (org_id);

CREATE INDEX idx_sheet_fields_page ON public.sheet_fields USING btree (page_id);

CREATE UNIQUE INDEX idx_sheet_fields_page_name_active ON public.sheet_fields USING btree (page_id, name) WHERE (archived_at IS NULL);

CREATE INDEX idx_sheet_import_jobs_org ON public.sheet_import_jobs USING btree (org_id);

CREATE INDEX idx_sheet_import_jobs_page ON public.sheet_import_jobs USING btree (page_id);

CREATE INDEX idx_sheet_operations_org ON public.sheet_operations USING btree (org_id);

CREATE INDEX idx_sheet_operations_page_created ON public.sheet_operations USING btree (page_id, created_at DESC);

CREATE INDEX idx_sheet_pages_org ON public.sheet_pages USING btree (org_id);

CREATE UNIQUE INDEX idx_sheet_pages_sheet_name_active ON public.sheet_pages USING btree (sheet_id, name) WHERE (archived_at IS NULL);

CREATE INDEX idx_sheet_rows_data_gin ON public.sheet_rows USING gin (data jsonb_path_ops);

CREATE INDEX idx_sheet_rows_import_job ON public.sheet_rows USING btree (import_job_id) WHERE (import_job_id IS NOT NULL);

CREATE INDEX idx_sheet_rows_page_created ON public.sheet_rows USING btree (page_id, created_at, id);

CREATE INDEX idx_sheet_rows_page_position_active ON public.sheet_rows USING btree (page_id, "position") WHERE (archived_at IS NULL);

CREATE INDEX idx_sheet_views_org ON public.sheet_views USING btree (org_id);

CREATE INDEX idx_sheet_views_page_active ON public.sheet_views USING btree (page_id) WHERE (archived_at IS NULL);

CREATE INDEX idx_sheets_team_updated_active ON public.sheets USING btree (team_id, updated_at DESC) WHERE (archived_at IS NULL);

CREATE UNIQUE INDEX idx_sheets_org_slug_active ON public.sheets USING btree (org_id, slug) WHERE (archived_at IS NULL);

CREATE INDEX idx_sheets_org_updated_active ON public.sheets USING btree (org_id, updated_at DESC) WHERE (archived_at IS NULL);

CREATE INDEX idx_skills_category ON public.skills USING btree (category);

CREATE INDEX idx_skills_hidden ON public.skills USING btree (hidden);

CREATE INDEX idx_skills_org_id ON public.skills USING btree (org_id);

CREATE UNIQUE INDEX idx_skills_org_slug ON public.skills USING btree (org_id, slug) WHERE (team_id IS NULL);

CREATE INDEX idx_skills_publisher_id ON public.skills USING btree (publisher_id);

CREATE INDEX idx_skills_slug ON public.skills USING btree (slug);

CREATE INDEX idx_skills_status ON public.skills USING btree (status);

CREATE INDEX idx_skills_team_id ON public.skills USING btree (team_id);

CREATE UNIQUE INDEX idx_skills_team_slug ON public.skills USING btree (team_id, slug) WHERE (team_id IS NOT NULL);

CREATE UNIQUE INDEX idx_slack_thread_events_connection_event ON public.slack_thread_events USING btree (connection_id, event_id) WHERE (event_id <> ''::text);

CREATE INDEX idx_slack_thread_events_session ON public.slack_thread_events USING btree (session_id);

CREATE INDEX idx_slack_thread_events_thread_direction ON public.slack_thread_events USING btree (org_id, connection_id, slack_channel_id, thread_ts, direction, message_at DESC);

CREATE INDEX idx_team_external_resource_routes_org ON public.team_external_resource_routes USING btree (org_id);

CREATE INDEX idx_team_external_resource_routes_team ON public.team_external_resource_routes USING btree (team_id);

CREATE INDEX idx_team_external_resource_routes_agent ON public.team_external_resource_routes USING btree (agent_id);

CREATE UNIQUE INDEX idx_team_external_resource_routes_resource ON public.team_external_resource_routes USING btree (connection_id, resource_type, resource_key);

CREATE INDEX idx_slack_thread_events_trigger_id ON public.slack_thread_events USING btree (trigger_id);

CREATE INDEX idx_team_connection_grants_org_team ON public.team_connection_grants USING btree (org_id, team_id);

CREATE INDEX idx_team_env_vars_org ON public.team_env_vars USING btree (org_id);

CREATE INDEX idx_team_env_vars_team ON public.team_env_vars USING btree (team_id);

CREATE UNIQUE INDEX idx_team_env_vars_team_name ON public.team_env_vars USING btree (team_id, name);

CREATE INDEX idx_team_mcp_servers_org_team ON public.team_mcp_servers USING btree (org_id, team_id);

CREATE INDEX idx_team_mcp_servers_server ON public.team_mcp_servers USING btree (mcp_server_id);

CREATE INDEX idx_team_members_org_user ON public.team_members USING btree (org_id, user_id);

CREATE INDEX idx_team_members_team_id ON public.team_members USING btree (team_id);

CREATE UNIQUE INDEX idx_team_members_team_user ON public.team_members USING btree (team_id, user_id);

CREATE INDEX idx_team_rag_sources_org_team ON public.team_rag_sources USING btree (org_id, team_id);

CREATE INDEX idx_team_skill_grants_org_team ON public.team_skill_grants USING btree (org_id, team_id);

CREATE INDEX idx_teams_archived_at ON public.teams USING btree (archived_at);

CREATE UNIQUE INDEX idx_teams_id_org_id ON public.teams USING btree (id, org_id);

CREATE INDEX idx_teams_org_id ON public.teams USING btree (org_id);

CREATE UNIQUE INDEX idx_teams_org_name_active ON public.teams USING btree (org_id, name) WHERE (archived_at IS NULL);

CREATE INDEX idx_tokens_credential_id ON public.tokens USING btree (credential_id);

CREATE UNIQUE INDEX idx_tokens_jti ON public.tokens USING btree (jti);

CREATE INDEX idx_trigger_delivery_org_agent_created ON public.agent_trigger_deliveries USING btree (org_id, agent_id, created_at);

CREATE INDEX idx_trigger_delivery_org_agent_session_created ON public.agent_trigger_deliveries USING btree (org_id, agent_id, runtime_session_id, created_at);

CREATE INDEX idx_tu_org_agent ON public.tool_usages USING btree (agent_id);

CREATE INDEX idx_tu_org_created ON public.tool_usages USING btree (org_id, created_at);

CREATE UNIQUE INDEX idx_usage_unique ON public.usage USING btree (org_id, credential_id, period_start);

CREATE INDEX idx_user_agent_mcp_servers_agent ON public.user_agent_mcp_servers USING btree (agent_id);

CREATE INDEX idx_user_agent_mcp_servers_org ON public.user_agent_mcp_servers USING btree (org_id);

CREATE INDEX idx_user_agent_mcp_servers_server ON public.user_agent_mcp_servers USING btree (mcp_server_id);

CREATE UNIQUE INDEX idx_users_email ON public.users USING btree (email);

CREATE UNIQUE INDEX team_connection_grants_team_connection_unique ON public.team_connection_grants USING btree (team_id, connection_id) WHERE (connection_id IS NOT NULL);

CREATE UNIQUE INDEX team_connection_grants_team_database_connection_unique ON public.team_connection_grants USING btree (team_id, database_connection_id) WHERE (database_connection_id IS NOT NULL);

CREATE UNIQUE INDEX uq_rag_sync_state_rag_source_id ON public.rag_sync_states USING btree (rag_source_id);

CREATE TRIGGER agent_mcp_servers_config_version AFTER INSERT OR DELETE OR UPDATE ON public.agent_mcp_servers FOR EACH ROW EXECUTE FUNCTION public.bump_mcp_config_version();

CREATE TRIGGER agents_mcp_config_version AFTER UPDATE OF team_id, status ON public.agents FOR EACH ROW EXECUTE FUNCTION public.bump_mcp_config_version();

CREATE TRIGGER connections_mcp_config_version AFTER INSERT OR DELETE OR UPDATE ON public.connections FOR EACH ROW EXECUTE FUNCTION public.bump_mcp_config_version();

CREATE TRIGGER database_connections_mcp_config_version AFTER INSERT OR DELETE OR UPDATE ON public.database_connections FOR EACH ROW EXECUTE FUNCTION public.bump_mcp_config_version();

CREATE TRIGGER mcp_authorizations_config_version AFTER INSERT OR DELETE OR UPDATE ON public.mcp_authorizations FOR EACH ROW EXECUTE FUNCTION public.bump_mcp_config_version();

CREATE TRIGGER mcp_servers_config_version AFTER INSERT OR DELETE OR UPDATE ON public.mcp_servers FOR EACH ROW EXECUTE FUNCTION public.bump_mcp_config_version();

CREATE TRIGGER org_memberships_mcp_config_version AFTER INSERT OR DELETE OR UPDATE ON public.org_memberships FOR EACH ROW EXECUTE FUNCTION public.bump_mcp_config_version();

CREATE TRIGGER team_connection_grants_mcp_config_version AFTER INSERT OR DELETE OR UPDATE ON public.team_connection_grants FOR EACH ROW EXECUTE FUNCTION public.bump_mcp_config_version();

CREATE TRIGGER team_mcp_servers_config_version AFTER INSERT OR DELETE OR UPDATE ON public.team_mcp_servers FOR EACH ROW EXECUTE FUNCTION public.bump_mcp_config_version();

CREATE TRIGGER team_members_mcp_config_version AFTER INSERT OR DELETE OR UPDATE ON public.team_members FOR EACH ROW EXECUTE FUNCTION public.bump_mcp_config_version();

CREATE TRIGGER teams_mcp_config_version AFTER UPDATE OF archived_at ON public.teams FOR EACH ROW EXECUTE FUNCTION public.bump_mcp_config_version();

CREATE TRIGGER user_agent_mcp_servers_config_version AFTER INSERT OR DELETE OR UPDATE ON public.user_agent_mcp_servers FOR EACH ROW EXECUTE FUNCTION public.bump_mcp_config_version();

ALTER TABLE ONLY public.agent_directives
    ADD CONSTRAINT agent_directives_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_directives
    ADD CONSTRAINT agent_directives_created_by_user_id_fkey FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.agent_directives
    ADD CONSTRAINT agent_directives_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_mcp_servers
    ADD CONSTRAINT agent_mcp_servers_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_mcp_servers
    ADD CONSTRAINT agent_mcp_servers_agent_org_fkey FOREIGN KEY (agent_id, org_id) REFERENCES public.agents(id, org_id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_mcp_servers
    ADD CONSTRAINT agent_mcp_servers_mcp_server_id_fkey FOREIGN KEY (mcp_server_id) REFERENCES public.mcp_servers(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_mcp_servers
    ADD CONSTRAINT agent_mcp_servers_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_mcp_servers
    ADD CONSTRAINT agent_mcp_servers_server_org_fkey FOREIGN KEY (mcp_server_id, org_id) REFERENCES public.mcp_servers(id, org_id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_mcp_servers
    ADD CONSTRAINT agent_mcp_servers_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.agent_schedules
    ADD CONSTRAINT agent_schedules_created_by_user_id_fkey FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT agents_team_id_fkey FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE RESTRICT;

ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT api_keys_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.app_versions
    ADD CONSTRAINT app_versions_app_id_fkey FOREIGN KEY (app_id) REFERENCES public.apps(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.app_versions
    ADD CONSTRAINT app_versions_created_by_agent_id_fkey FOREIGN KEY (created_by_agent_id) REFERENCES public.agents(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.app_versions
    ADD CONSTRAINT app_versions_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.app_versions
    ADD CONSTRAINT app_versions_source_session_id_fkey FOREIGN KEY (source_session_id) REFERENCES public.sessions(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.apps
    ADD CONSTRAINT apps_team_id_fkey FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE RESTRICT;

ALTER TABLE ONLY public.apps
    ADD CONSTRAINT apps_created_by_agent_id_fkey FOREIGN KEY (created_by_agent_id) REFERENCES public.agents(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.apps
    ADD CONSTRAINT apps_created_by_user_id_fkey FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.apps
    ADD CONSTRAINT apps_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.apps
    ADD CONSTRAINT apps_sandbox_id_fkey FOREIGN KEY (sandbox_id) REFERENCES public.sandboxes(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.apps
    ADD CONSTRAINT apps_sheet_id_fkey FOREIGN KEY (sheet_id) REFERENCES public.sheets(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.apps
    ADD CONSTRAINT apps_source_session_id_fkey FOREIGN KEY (source_session_id) REFERENCES public.sessions(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.canvas_artifact_files
    ADD CONSTRAINT canvas_artifact_files_canvas_artifact_id_fkey FOREIGN KEY (canvas_artifact_id) REFERENCES public.canvas_artifacts(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.canvas_artifact_files
    ADD CONSTRAINT canvas_artifact_files_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.canvas_artifacts
    ADD CONSTRAINT canvas_artifacts_canvas_project_id_fkey FOREIGN KEY (canvas_project_id) REFERENCES public.canvas_projects(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.canvas_artifacts
    ADD CONSTRAINT canvas_artifacts_created_by_agent_id_fkey FOREIGN KEY (created_by_agent_id) REFERENCES public.agents(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.canvas_artifacts
    ADD CONSTRAINT canvas_artifacts_created_by_user_id_fkey FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.canvas_artifacts
    ADD CONSTRAINT canvas_artifacts_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.canvas_artifacts
    ADD CONSTRAINT canvas_artifacts_source_session_id_fkey FOREIGN KEY (source_session_id) REFERENCES public.sessions(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.canvas_projects
    ADD CONSTRAINT canvas_projects_created_by_agent_id_fkey FOREIGN KEY (created_by_agent_id) REFERENCES public.agents(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.canvas_projects
    ADD CONSTRAINT canvas_projects_created_by_user_id_fkey FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.canvas_projects
    ADD CONSTRAINT canvas_projects_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_assets
    ADD CONSTRAINT fk_agent_assets_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_assets
    ADD CONSTRAINT fk_agent_assets_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_assets
    ADD CONSTRAINT fk_agent_assets_sandbox FOREIGN KEY (sandbox_id) REFERENCES public.sandboxes(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_memory_digests
    ADD CONSTRAINT fk_agent_memory_digests_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_memory_digests
    ADD CONSTRAINT fk_agent_memory_digests_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_email_threads
    ADD CONSTRAINT fk_agent_email_threads_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_email_threads
    ADD CONSTRAINT fk_agent_email_threads_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_email_threads
    ADD CONSTRAINT fk_agent_email_threads_session FOREIGN KEY (session_id) REFERENCES public.sessions(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.agent_email_messages
    ADD CONSTRAINT fk_agent_email_messages_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_email_messages
    ADD CONSTRAINT fk_agent_email_messages_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_email_messages
    ADD CONSTRAINT fk_agent_email_messages_thread FOREIGN KEY (thread_id) REFERENCES public.agent_email_threads(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_memories
    ADD CONSTRAINT fk_agent_memories_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_memories
    ADD CONSTRAINT fk_agent_memories_created_by FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.agent_memories
    ADD CONSTRAINT fk_agent_memories_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_memories
    ADD CONSTRAINT fk_agent_memories_source_event FOREIGN KEY (source_event_id) REFERENCES public.session_events(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.agent_memories
    ADD CONSTRAINT fk_agent_memories_source_session FOREIGN KEY (source_session_id) REFERENCES public.sessions(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.agent_observations
    ADD CONSTRAINT fk_agent_observations_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_observations
    ADD CONSTRAINT fk_agent_observations_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_observations
    ADD CONSTRAINT fk_agent_observations_superseded_by FOREIGN KEY (superseded_by) REFERENCES public.agent_observations(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.agent_schedule_runs
    ADD CONSTRAINT fk_agent_schedule_runs_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_schedule_runs
    ADD CONSTRAINT fk_agent_schedule_runs_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_schedule_runs
    ADD CONSTRAINT fk_agent_schedule_runs_sandbox FOREIGN KEY (sandbox_id) REFERENCES public.sandboxes(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_schedule_runs
    ADD CONSTRAINT fk_agent_schedule_runs_schedule FOREIGN KEY (schedule_id) REFERENCES public.agent_schedules(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_schedule_runs
    ADD CONSTRAINT fk_agent_schedule_runs_session FOREIGN KEY (session_id) REFERENCES public.sessions(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.agent_schedules
    ADD CONSTRAINT fk_agent_schedules_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_schedules
    ADD CONSTRAINT fk_agent_schedules_connection FOREIGN KEY (connection_id) REFERENCES public.connections(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.agent_schedules
    ADD CONSTRAINT fk_agent_schedules_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_schedules
    ADD CONSTRAINT fk_agent_schedules_sandbox FOREIGN KEY (sandbox_id) REFERENCES public.sandboxes(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.agent_trigger_deliveries
    ADD CONSTRAINT fk_agent_trigger_deliveries_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_trigger_deliveries
    ADD CONSTRAINT fk_agent_trigger_deliveries_connection FOREIGN KEY (connection_id) REFERENCES public.connections(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.agent_trigger_deliveries
    ADD CONSTRAINT fk_agent_trigger_deliveries_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_trigger_deliveries
    ADD CONSTRAINT fk_agent_trigger_deliveries_session FOREIGN KEY (session_id) REFERENCES public.sessions(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_trigger_deliveries
    ADD CONSTRAINT fk_agent_trigger_deliveries_trigger FOREIGN KEY (trigger_id) REFERENCES public.agent_triggers(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_triggers
    ADD CONSTRAINT fk_agent_triggers_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_triggers
    ADD CONSTRAINT fk_agent_triggers_connection FOREIGN KEY (connection_id) REFERENCES public.connections(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_triggers
    ADD CONSTRAINT fk_agent_triggers_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT fk_agents_agent_catalog FOREIGN KEY (agent_catalog_id) REFERENCES public.agent_catalog(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT fk_agents_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT fk_agents_parent_agent FOREIGN KEY (parent_agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT fk_agents_sandbox_template FOREIGN KEY (sandbox_template_id) REFERENCES public.sandbox_templates(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT fk_api_keys_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.apps
    ADD CONSTRAINT fk_apps_active_version FOREIGN KEY (active_version_id) REFERENCES public.app_versions(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.billing_payment_methods
    ADD CONSTRAINT fk_billing_payment_methods_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.billing_payment_methods
    ADD CONSTRAINT fk_billing_payment_methods_user FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.credit_purchases
    ADD CONSTRAINT fk_credit_purchases_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.credit_purchases
    ADD CONSTRAINT fk_credit_purchases_created_by_user FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.credit_purchases
    ADD CONSTRAINT fk_credit_purchases_payment_method FOREIGN KEY (payment_method_id) REFERENCES public.billing_payment_methods(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.brand_assets
    ADD CONSTRAINT fk_brand_assets_brand FOREIGN KEY (brand_id) REFERENCES public.brands(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.brand_assets
    ADD CONSTRAINT fk_brand_assets_created_by FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.brand_assets
    ADD CONSTRAINT fk_brand_assets_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.brands
    ADD CONSTRAINT fk_brands_created_by FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.brands
    ADD CONSTRAINT fk_brands_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.connections
    ADD CONSTRAINT fk_connections_integration FOREIGN KEY (integration_id) REFERENCES public.integrations(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.connections
    ADD CONSTRAINT fk_connections_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.connections
    ADD CONSTRAINT fk_connections_user FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.credentials
    ADD CONSTRAINT fk_credentials_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.database_connections
    ADD CONSTRAINT fk_database_connections_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.drive_assets
    ADD CONSTRAINT fk_drive_assets_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.drive_assets
    ADD CONSTRAINT fk_drive_assets_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.memory_suppressions
    ADD CONSTRAINT fk_memory_suppressions_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.memory_suppressions
    ADD CONSTRAINT fk_memory_suppressions_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.oauth_accounts
    ADD CONSTRAINT fk_oauth_accounts_user FOREIGN KEY (user_id) REFERENCES public.users(id);

ALTER TABLE ONLY public.org_invite_teams
    ADD CONSTRAINT fk_org_invite_teams_invite FOREIGN KEY (org_invite_id) REFERENCES public.org_invites(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.org_invite_teams
    ADD CONSTRAINT fk_org_invite_teams_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.org_invite_teams
    ADD CONSTRAINT fk_org_invite_teams_team FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.org_invites
    ADD CONSTRAINT fk_org_invites_invited_by FOREIGN KEY (invited_by_id) REFERENCES public.users(id);

ALTER TABLE ONLY public.org_invites
    ADD CONSTRAINT fk_org_invites_org FOREIGN KEY (org_id) REFERENCES public.orgs(id);

ALTER TABLE ONLY public.org_memberships
    ADD CONSTRAINT fk_org_memberships_org FOREIGN KEY (org_id) REFERENCES public.orgs(id);

ALTER TABLE ONLY public.org_memberships
    ADD CONSTRAINT fk_org_memberships_user FOREIGN KEY (user_id) REFERENCES public.users(id);

ALTER TABLE ONLY public.rag_index_attempt_errors
    ADD CONSTRAINT fk_rag_index_attempt_errors_index_attempt FOREIGN KEY (index_attempt_id) REFERENCES public.rag_index_attempts(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.rag_sources
    ADD CONSTRAINT fk_rag_sources_connection FOREIGN KEY (connection_id) REFERENCES public.connections(id);

ALTER TABLE ONLY public.sandbox_templates
    ADD CONSTRAINT fk_sandbox_templates_base_template FOREIGN KEY (base_template_id) REFERENCES public.sandbox_templates(id);

ALTER TABLE ONLY public.sandbox_templates
    ADD CONSTRAINT fk_sandbox_templates_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sandbox_warm_slots
    ADD CONSTRAINT fk_sandbox_warm_slots_claimed_sandbox FOREIGN KEY (claimed_sandbox_id) REFERENCES public.sandboxes(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.sandboxes
    ADD CONSTRAINT fk_sandboxes_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sandboxes
    ADD CONSTRAINT fk_sandboxes_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sandboxes
    ADD CONSTRAINT fk_sandboxes_sandbox_template FOREIGN KEY (sandbox_template_id) REFERENCES public.sandbox_templates(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.session_events
    ADD CONSTRAINT fk_session_events_actor_user FOREIGN KEY (actor_user_id) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.session_events
    ADD CONSTRAINT fk_session_events_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE RESTRICT;

ALTER TABLE ONLY public.session_events
    ADD CONSTRAINT fk_session_events_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.session_events
    ADD CONSTRAINT fk_session_events_sandbox FOREIGN KEY (sandbox_id) REFERENCES public.sandboxes(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.session_events
    ADD CONSTRAINT fk_session_events_session FOREIGN KEY (session_id) REFERENCES public.sessions(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.session_message_queue
    ADD CONSTRAINT fk_session_message_queue_actor_user FOREIGN KEY (actor_user_id) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.session_message_queue
    ADD CONSTRAINT fk_session_message_queue_event FOREIGN KEY (session_event_id) REFERENCES public.session_events(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.session_message_queue
    ADD CONSTRAINT fk_session_message_queue_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.session_message_queue
    ADD CONSTRAINT fk_session_message_queue_session FOREIGN KEY (session_id) REFERENCES public.sessions(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.session_participants
    ADD CONSTRAINT fk_session_participants_inviter FOREIGN KEY (invited_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.session_participants
    ADD CONSTRAINT fk_session_participants_session FOREIGN KEY (session_id) REFERENCES public.sessions(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.session_participants
    ADD CONSTRAINT fk_session_participants_user FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.session_reflection_states
    ADD CONSTRAINT fk_session_reflection_states_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE RESTRICT;

ALTER TABLE ONLY public.session_reflection_states
    ADD CONSTRAINT fk_session_reflection_states_last_event FOREIGN KEY (last_reflected_event_id) REFERENCES public.session_events(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.session_reflection_states
    ADD CONSTRAINT fk_session_reflection_states_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.session_reflection_states
    ADD CONSTRAINT fk_session_reflection_states_session FOREIGN KEY (session_id) REFERENCES public.sessions(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT fk_sessions_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE RESTRICT;

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT fk_sessions_team FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE RESTRICT;

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT fk_sessions_created_by FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT fk_sessions_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT fk_sessions_sandbox FOREIGN KEY (sandbox_id) REFERENCES public.sandboxes(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.skills
    ADD CONSTRAINT fk_skills_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.skills
    ADD CONSTRAINT fk_skills_publisher FOREIGN KEY (publisher_id) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.skills
    ADD CONSTRAINT fk_skills_team FOREIGN KEY (team_id, org_id) REFERENCES public.teams(id, org_id) ON DELETE CASCADE;

ALTER TABLE ONLY public.slack_thread_events
    ADD CONSTRAINT fk_slack_thread_events_team FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.slack_thread_events
    ADD CONSTRAINT fk_slack_thread_events_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.slack_thread_events
    ADD CONSTRAINT fk_slack_thread_events_connection FOREIGN KEY (connection_id) REFERENCES public.connections(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.slack_thread_events
    ADD CONSTRAINT fk_slack_thread_events_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.slack_thread_events
    ADD CONSTRAINT fk_slack_thread_events_session FOREIGN KEY (session_id) REFERENCES public.sessions(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.slack_thread_events
    ADD CONSTRAINT fk_slack_thread_events_session_event FOREIGN KEY (session_event_id) REFERENCES public.session_events(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.slack_thread_events
    ADD CONSTRAINT fk_slack_thread_events_session_queue FOREIGN KEY (session_message_queue_id) REFERENCES public.session_message_queue(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.slack_thread_events
    ADD CONSTRAINT fk_slack_thread_events_trigger FOREIGN KEY (trigger_id) REFERENCES public.agent_triggers(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.team_members
    ADD CONSTRAINT fk_team_members_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.team_members
    ADD CONSTRAINT fk_team_members_team FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.team_members
    ADD CONSTRAINT fk_team_members_user FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.teams
    ADD CONSTRAINT fk_teams_creator FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.teams
    ADD CONSTRAINT fk_teams_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.tokens
    ADD CONSTRAINT fk_tokens_credential FOREIGN KEY (credential_id) REFERENCES public.credentials(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.tokens
    ADD CONSTRAINT fk_tokens_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.usage
    ADD CONSTRAINT fk_usage_credential FOREIGN KEY (credential_id) REFERENCES public.credentials(id);

ALTER TABLE ONLY public.usage
    ADD CONSTRAINT fk_usage_org FOREIGN KEY (org_id) REFERENCES public.orgs(id);

ALTER TABLE ONLY public.mcp_authorizations
    ADD CONSTRAINT mcp_authorizations_mcp_server_id_fkey FOREIGN KEY (mcp_server_id) REFERENCES public.mcp_servers(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.mcp_authorizations
    ADD CONSTRAINT mcp_authorizations_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.mcp_authorizations
    ADD CONSTRAINT mcp_authorizations_principal_user_id_fkey FOREIGN KEY (principal_user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.mcp_authorizations
    ADD CONSTRAINT mcp_authorizations_server_org_fkey FOREIGN KEY (mcp_server_id, org_id) REFERENCES public.mcp_servers(id, org_id) ON DELETE CASCADE;

ALTER TABLE ONLY public.mcp_oauth_states
    ADD CONSTRAINT mcp_oauth_states_mcp_server_id_fkey FOREIGN KEY (mcp_server_id) REFERENCES public.mcp_servers(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.mcp_oauth_states
    ADD CONSTRAINT mcp_oauth_states_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.mcp_oauth_states
    ADD CONSTRAINT mcp_oauth_states_server_org_fkey FOREIGN KEY (mcp_server_id, org_id) REFERENCES public.mcp_servers(id, org_id) ON DELETE CASCADE;

ALTER TABLE ONLY public.mcp_oauth_states
    ADD CONSTRAINT mcp_oauth_states_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.mcp_servers
    ADD CONSTRAINT mcp_servers_created_by_user_id_fkey FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.mcp_servers
    ADD CONSTRAINT mcp_servers_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.mcp_servers
    ADD CONSTRAINT mcp_servers_owner_user_id_fkey FOREIGN KEY (owner_user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT sessions_runtime_mcp_actor_user_id_fkey FOREIGN KEY (runtime_mcp_actor_user_id) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.sheet_fields
    ADD CONSTRAINT sheet_fields_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sheet_fields
    ADD CONSTRAINT sheet_fields_page_id_fkey FOREIGN KEY (page_id) REFERENCES public.sheet_pages(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sheet_import_jobs
    ADD CONSTRAINT sheet_import_jobs_created_by_agent_id_fkey FOREIGN KEY (created_by_agent_id) REFERENCES public.agents(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.sheet_import_jobs
    ADD CONSTRAINT sheet_import_jobs_created_by_user_id_fkey FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.sheet_import_jobs
    ADD CONSTRAINT sheet_import_jobs_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sheet_import_jobs
    ADD CONSTRAINT sheet_import_jobs_page_id_fkey FOREIGN KEY (page_id) REFERENCES public.sheet_pages(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sheet_operations
    ADD CONSTRAINT sheet_operations_actor_agent_id_fkey FOREIGN KEY (actor_agent_id) REFERENCES public.agents(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.sheet_operations
    ADD CONSTRAINT sheet_operations_actor_user_id_fkey FOREIGN KEY (actor_user_id) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.sheet_operations
    ADD CONSTRAINT sheet_operations_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sheet_operations
    ADD CONSTRAINT sheet_operations_page_id_fkey FOREIGN KEY (page_id) REFERENCES public.sheet_pages(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sheet_operations
    ADD CONSTRAINT sheet_operations_reverted_by_agent_id_fkey FOREIGN KEY (reverted_by_agent_id) REFERENCES public.agents(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.sheet_operations
    ADD CONSTRAINT sheet_operations_reverted_by_user_id_fkey FOREIGN KEY (reverted_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.sheet_operations
    ADD CONSTRAINT sheet_operations_source_session_id_fkey FOREIGN KEY (source_session_id) REFERENCES public.sessions(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.sheet_pages
    ADD CONSTRAINT sheet_pages_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sheet_pages
    ADD CONSTRAINT sheet_pages_sheet_id_fkey FOREIGN KEY (sheet_id) REFERENCES public.sheets(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sheet_rows
    ADD CONSTRAINT sheet_rows_created_by_agent_id_fkey FOREIGN KEY (created_by_agent_id) REFERENCES public.agents(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.sheet_rows
    ADD CONSTRAINT sheet_rows_created_by_user_id_fkey FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.sheet_rows
    ADD CONSTRAINT sheet_rows_import_job_id_fkey FOREIGN KEY (import_job_id) REFERENCES public.sheet_import_jobs(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.sheet_rows
    ADD CONSTRAINT sheet_rows_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sheet_rows
    ADD CONSTRAINT sheet_rows_page_id_fkey FOREIGN KEY (page_id) REFERENCES public.sheet_pages(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sheet_views
    ADD CONSTRAINT sheet_views_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sheet_views
    ADD CONSTRAINT sheet_views_page_id_fkey FOREIGN KEY (page_id) REFERENCES public.sheet_pages(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sheets
    ADD CONSTRAINT sheets_team_id_fkey FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE RESTRICT;

ALTER TABLE ONLY public.sheets
    ADD CONSTRAINT sheets_created_by_agent_id_fkey FOREIGN KEY (created_by_agent_id) REFERENCES public.agents(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.sheets
    ADD CONSTRAINT sheets_created_by_user_id_fkey FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.sheets
    ADD CONSTRAINT sheets_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sheets
    ADD CONSTRAINT sheets_source_session_id_fkey FOREIGN KEY (source_session_id) REFERENCES public.sessions(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.team_connection_grants
    ADD CONSTRAINT team_connection_grants_connection_org_fkey FOREIGN KEY (connection_id, org_id) REFERENCES public.connections(id, org_id) ON DELETE CASCADE;

ALTER TABLE ONLY public.team_connection_grants
    ADD CONSTRAINT team_connection_grants_database_connection_org_fkey FOREIGN KEY (database_connection_id, org_id) REFERENCES public.database_connections(id, org_id) ON DELETE CASCADE;

ALTER TABLE ONLY public.team_connection_grants
    ADD CONSTRAINT team_connection_grants_granted_by_fkey FOREIGN KEY (granted_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.team_connection_grants
    ADD CONSTRAINT team_connection_grants_team_org_fkey FOREIGN KEY (team_id, org_id) REFERENCES public.teams(id, org_id) ON DELETE CASCADE;

ALTER TABLE ONLY public.team_external_resource_routes
    ADD CONSTRAINT team_external_resource_routes_org_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.team_external_resource_routes
    ADD CONSTRAINT team_external_resource_routes_team_fkey FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.team_external_resource_routes
    ADD CONSTRAINT team_external_resource_routes_connection_fkey FOREIGN KEY (connection_id) REFERENCES public.connections(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.team_external_resource_routes
    ADD CONSTRAINT team_external_resource_routes_agent_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE RESTRICT;

ALTER TABLE ONLY public.team_external_resource_routes
    ADD CONSTRAINT team_external_resource_routes_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.team_env_vars
    ADD CONSTRAINT team_env_vars_team_org_fkey FOREIGN KEY (team_id, org_id) REFERENCES public.teams(id, org_id) ON DELETE CASCADE;

ALTER TABLE ONLY public.team_mcp_servers
    ADD CONSTRAINT team_mcp_servers_granted_by_fkey FOREIGN KEY (granted_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.team_mcp_servers
    ADD CONSTRAINT team_mcp_servers_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.team_mcp_servers
    ADD CONSTRAINT team_mcp_servers_server_org_fkey FOREIGN KEY (mcp_server_id, org_id) REFERENCES public.mcp_servers(id, org_id) ON DELETE CASCADE;

ALTER TABLE ONLY public.team_mcp_servers
    ADD CONSTRAINT team_mcp_servers_team_org_fkey FOREIGN KEY (team_id, org_id) REFERENCES public.teams(id, org_id) ON DELETE CASCADE;

ALTER TABLE ONLY public.team_rag_sources
    ADD CONSTRAINT team_rag_sources_granted_by_fkey FOREIGN KEY (granted_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.team_rag_sources
    ADD CONSTRAINT team_rag_sources_rag_source_id_fkey FOREIGN KEY (rag_source_id) REFERENCES public.rag_sources(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.team_rag_sources
    ADD CONSTRAINT team_rag_sources_team_id_fkey FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.team_skill_grants
    ADD CONSTRAINT team_skill_grants_granted_by_fkey FOREIGN KEY (granted_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.team_skill_grants
    ADD CONSTRAINT team_skill_grants_skill_fkey FOREIGN KEY (skill_id) REFERENCES public.skills(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.team_skill_grants
    ADD CONSTRAINT team_skill_grants_team_org_fkey FOREIGN KEY (team_id, org_id) REFERENCES public.teams(id, org_id) ON DELETE CASCADE;

ALTER TABLE ONLY public.user_agent_mcp_servers
    ADD CONSTRAINT user_agent_mcp_servers_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.user_agent_mcp_servers
    ADD CONSTRAINT user_agent_mcp_servers_agent_org_fkey FOREIGN KEY (agent_id, org_id) REFERENCES public.agents(id, org_id) ON DELETE CASCADE;

ALTER TABLE ONLY public.user_agent_mcp_servers
    ADD CONSTRAINT user_agent_mcp_servers_mcp_server_id_fkey FOREIGN KEY (mcp_server_id) REFERENCES public.mcp_servers(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.user_agent_mcp_servers
    ADD CONSTRAINT user_agent_mcp_servers_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.user_agent_mcp_servers
    ADD CONSTRAINT user_agent_mcp_servers_server_org_fkey FOREIGN KEY (mcp_server_id, org_id) REFERENCES public.mcp_servers(id, org_id) ON DELETE CASCADE;

ALTER TABLE ONLY public.user_agent_mcp_servers
    ADD CONSTRAINT user_agent_mcp_servers_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;
