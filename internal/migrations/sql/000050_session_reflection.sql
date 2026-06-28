-- +goose Up
ALTER TABLE agent_memories
    ADD COLUMN IF NOT EXISTS memory_fingerprint text DEFAULT ''::text NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_memories_fingerprint
    ON agent_memories (
        org_id,
        scope,
        COALESCE(user_id, '00000000-0000-0000-0000-000000000000'::uuid),
        COALESCE(agent_id, '00000000-0000-0000-0000-000000000000'::uuid),
        memory_fingerprint
    )
    WHERE archived_at IS NULL AND memory_fingerprint <> '';

CREATE TABLE session_reflection_states (
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
    CONSTRAINT session_reflection_states_status_check CHECK (status IN ('idle', 'running', 'failed'))
);

ALTER TABLE ONLY session_reflection_states
    ADD CONSTRAINT session_reflection_states_pkey PRIMARY KEY (session_id);
ALTER TABLE ONLY session_reflection_states
    ADD CONSTRAINT fk_session_reflection_states_session FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE;
ALTER TABLE ONLY session_reflection_states
    ADD CONSTRAINT fk_session_reflection_states_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;
ALTER TABLE ONLY session_reflection_states
    ADD CONSTRAINT fk_session_reflection_states_agent FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE RESTRICT;
ALTER TABLE ONLY session_reflection_states
    ADD CONSTRAINT fk_session_reflection_states_last_event FOREIGN KEY (last_reflected_event_id) REFERENCES session_events(id) ON DELETE SET NULL;

CREATE INDEX idx_session_reflection_states_scan
    ON session_reflection_states USING btree (status, locked_until, updated_at);

-- +goose Down
DROP TABLE IF EXISTS session_reflection_states;
DROP INDEX IF EXISTS idx_agent_memories_fingerprint;
ALTER TABLE agent_memories DROP COLUMN IF EXISTS memory_fingerprint;
