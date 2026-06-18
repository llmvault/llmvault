-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS microsandbox_runners (
    id text PRIMARY KEY,
    name text NOT NULL UNIQUE,
    api_url text NOT NULL,
    auth_token_hash bytea NOT NULL,
    status text NOT NULL,
    drain boolean NOT NULL DEFAULT false,
    total_cpu bigint NOT NULL,
    total_memory_mb bigint NOT NULL,
    total_disk_gb bigint NOT NULL,
    reserved_cpu bigint NOT NULL DEFAULT 0,
    reserved_memory_mb bigint NOT NULL DEFAULT 0,
    reserved_disk_gb bigint NOT NULL DEFAULT 0,
    cpu_overcommit double precision NOT NULL DEFAULT 1.5,
    memory_overcommit double precision NOT NULL DEFAULT 1,
    disk_overcommit double precision NOT NULL DEFAULT 1,
    metadata_json text NOT NULL DEFAULT '{}',
    last_heartbeat_at timestamptz,
    created_at timestamptz,
    updated_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_microsandbox_runners_status ON microsandbox_runners (status);

CREATE TABLE IF NOT EXISTS microsandbox_org_preview_secrets (
    org_id text PRIMARY KEY,
    password_ciphertext bytea NOT NULL,
    created_at timestamptz,
    updated_at timestamptz
);

CREATE TABLE IF NOT EXISTS microsandbox_sandboxes (
    id text PRIMARY KEY,
    org_id text NOT NULL,
    runner_id text NOT NULL,
    name text NOT NULL,
    image_ref text NOT NULL,
    snapshot_id text,
    status text NOT NULL,
    cpu bigint NOT NULL,
    memory_mb bigint NOT NULL,
    disk_gb bigint NOT NULL,
    metadata_json text NOT NULL DEFAULT '{}',
    error_message text NOT NULL DEFAULT '',
    created_at timestamptz,
    updated_at timestamptz,
    stopped_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_microsandbox_sandboxes_org_id ON microsandbox_sandboxes (org_id);
CREATE INDEX IF NOT EXISTS idx_microsandbox_sandboxes_runner_id ON microsandbox_sandboxes (runner_id);
CREATE INDEX IF NOT EXISTS idx_microsandbox_sandboxes_snapshot_id ON microsandbox_sandboxes (snapshot_id);
CREATE INDEX IF NOT EXISTS idx_microsandbox_sandboxes_status ON microsandbox_sandboxes (status);

CREATE TABLE IF NOT EXISTS microsandbox_sandbox_ports (
    id text PRIMARY KEY,
    sandbox_id text NOT NULL,
    guest_port bigint NOT NULL,
    host_port bigint NOT NULL,
    protocol text NOT NULL DEFAULT 'http',
    created_at timestamptz,
    updated_at timestamptz
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_microsandbox_sandbox_ports_sandbox_guest
    ON microsandbox_sandbox_ports (sandbox_id, guest_port);
CREATE INDEX IF NOT EXISTS idx_microsandbox_sandbox_ports_sandbox_id
    ON microsandbox_sandbox_ports (sandbox_id);

CREATE TABLE IF NOT EXISTS microsandbox_snapshots (
    id text PRIMARY KEY,
    org_id text NOT NULL,
    runner_id text NOT NULL,
    name text NOT NULL,
    base_image_ref text NOT NULL,
    status text NOT NULL,
    artifact_url text NOT NULL DEFAULT '',
    commands_json text NOT NULL DEFAULT '[]',
    logs text NOT NULL DEFAULT '',
    error_message text NOT NULL DEFAULT '',
    created_at timestamptz,
    updated_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_microsandbox_snapshots_org_id ON microsandbox_snapshots (org_id);
CREATE INDEX IF NOT EXISTS idx_microsandbox_snapshots_runner_id ON microsandbox_snapshots (runner_id);
CREATE INDEX IF NOT EXISTS idx_microsandbox_snapshots_status ON microsandbox_snapshots (status);

CREATE TABLE IF NOT EXISTS microsandbox_events (
    id text PRIMARY KEY,
    kind text NOT NULL,
    runner_id text,
    sandbox_id text,
    message text NOT NULL,
    created_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_microsandbox_events_kind ON microsandbox_events (kind);
CREATE INDEX IF NOT EXISTS idx_microsandbox_events_runner_id ON microsandbox_events (runner_id);
CREATE INDEX IF NOT EXISTS idx_microsandbox_events_sandbox_id ON microsandbox_events (sandbox_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'microsandbox down migration is intentionally unsupported; reset or restore the microsandbox database instead'; END $$;
-- +goose StatementEnd
