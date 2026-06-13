CREATE TABLE agent_config (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    definition_json TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    created_at TEXT NOT NULL,
    last_activity_at TEXT NOT NULL
);
CREATE INDEX idx_sessions_activity ON sessions(last_activity_at DESC);
CREATE INDEX idx_sessions_status ON sessions(status);
CREATE INDEX idx_sessions_activity_id ON sessions(last_activity_at DESC, id DESC);
CREATE INDEX idx_sessions_status_activity_id ON sessions(status, last_activity_at DESC, id DESC);

CREATE TABLE session_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    seq INTEGER NOT NULL,
    kind TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE UNIQUE INDEX idx_session_events_session_seq ON session_events(session_id, seq);
CREATE INDEX idx_session_events_session_created ON session_events(session_id, created_at DESC);

CREATE TABLE event_idempotency_keys (
    key TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    event_id INTEGER,
    created_at TEXT NOT NULL
);
CREATE INDEX idx_event_idempotency_session
ON event_idempotency_keys(session_id, created_at DESC);

CREATE VIRTUAL TABLE IF NOT EXISTS session_event_search USING fts5(
    event_id UNINDEXED,
    session_id UNINDEXED,
    kind UNINDEXED,
    content,
    created_at UNINDEXED
);

CREATE TABLE inbound_dedupe (
    envelope_id TEXT PRIMARY KEY,
    received_at TEXT NOT NULL
);
CREATE INDEX idx_dedupe_received ON inbound_dedupe(received_at);

CREATE TABLE events_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_type TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    occurred_at TEXT NOT NULL,
    recorded_at TEXT NOT NULL
);
CREATE INDEX idx_events_log_type ON events_log(event_type, recorded_at DESC);
CREATE INDEX idx_events_log_recorded ON events_log(recorded_at DESC);

CREATE TABLE outbound_outbox (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    channel_name TEXT NOT NULL,
    event_type TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    next_retry_at TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX idx_outbox_due ON outbound_outbox(status, next_retry_at);
CREATE INDEX idx_outbox_channel ON outbound_outbox(channel_name);

CREATE TABLE cron_jobs (
    id TEXT PRIMARY KEY NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    channel TEXT NOT NULL,
    task_prompt TEXT NOT NULL,
    cron_expression TEXT,
    interval_seconds INTEGER,
    repeat_count INTEGER,
    repeat_completed INTEGER NOT NULL DEFAULT 0,
    state TEXT NOT NULL DEFAULT 'active',
    next_run_at TEXT NOT NULL,
    last_run_at TEXT,
    last_status TEXT,
    last_error TEXT,
    session_continuation_id TEXT,
    created_at TEXT NOT NULL,
    created_by_session TEXT NOT NULL
);
CREATE INDEX idx_cron_due ON cron_jobs(state, next_run_at);

CREATE TABLE subagent_tasks (
    id TEXT PRIMARY KEY NOT NULL,
    parent_session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    child_session_id TEXT NOT NULL,
    agent_name TEXT NOT NULL,
    goal TEXT NOT NULL,
    stream_id TEXT,
    state TEXT NOT NULL,
    result TEXT,
    error TEXT,
    created_at TEXT NOT NULL,
    started_at TEXT,
    completed_at TEXT,
    updated_at TEXT NOT NULL
);
CREATE INDEX idx_subagent_tasks_parent_state ON subagent_tasks(parent_session_id, state);
CREATE INDEX idx_subagent_tasks_state_created ON subagent_tasks(state, created_at);
