-- +goose Up
-- First-class sessions, participants, events, and message queue

CREATE TABLE sessions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    channel_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    sandbox_id uuid,
    created_by uuid,
    model text,
    access_mode text DEFAULT 'full'::text NOT NULL,
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
    ended_at timestamp with time zone
);

CREATE TABLE session_participants (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    session_id uuid NOT NULL,
    user_id uuid NOT NULL,
    role text DEFAULT 'collaborator'::text NOT NULL,
    invited_by uuid,
    joined_at timestamp with time zone,
    last_seen_at timestamp with time zone,
    created_at timestamp with time zone
);

CREATE TABLE session_events (
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
    created_at timestamp with time zone
);

CREATE TABLE session_message_queue (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    session_id uuid NOT NULL,
    session_event_id uuid NOT NULL,
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
    updated_at timestamp with time zone
);

ALTER TABLE ONLY sessions ADD CONSTRAINT sessions_pkey PRIMARY KEY (id);
ALTER TABLE ONLY session_participants ADD CONSTRAINT session_participants_pkey PRIMARY KEY (id);
ALTER TABLE ONLY session_events ADD CONSTRAINT session_events_pkey PRIMARY KEY (id);
ALTER TABLE ONLY session_message_queue ADD CONSTRAINT session_message_queue_pkey PRIMARY KEY (id);

CREATE INDEX idx_sessions_channel ON sessions USING btree (channel_id, created_at DESC);
CREATE INDEX idx_sessions_agent ON sessions USING btree (org_id, agent_id, created_at DESC);
CREATE INDEX idx_sessions_sandbox_id ON sessions USING btree (sandbox_id);
CREATE INDEX idx_sessions_agent_turn_status ON sessions USING btree (agent_turn_status) WHERE agent_turn_status <> 'idle';
CREATE UNIQUE INDEX idx_session_participants_session_user ON session_participants USING btree (session_id, user_id);
CREATE INDEX idx_session_participants_user_id ON session_participants USING btree (user_id);
CREATE UNIQUE INDEX idx_session_events_idem ON session_events USING btree (session_id, event_id) WHERE event_id IS NOT NULL;
CREATE INDEX idx_session_events_session ON session_events USING btree (session_id, event_at);
CREATE UNIQUE INDEX idx_session_message_queue_session_event ON session_message_queue USING btree (session_id, session_event_id);
CREATE UNIQUE INDEX idx_session_message_queue_sequence ON session_message_queue USING btree (session_id, sequence_number);
CREATE INDEX idx_session_message_queue_claim ON session_message_queue USING btree (session_id, status, sequence_number);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'baseline down migration is intentionally unsupported; reset or restore the database instead'; END $$;
-- +goose StatementEnd
