-- +goose Up
CREATE EXTENSION IF NOT EXISTS "vector";

CREATE TABLE agent_memories (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    agent_id uuid,
    user_id uuid,
    scope text NOT NULL,
    content text NOT NULL,
    tags text[] DEFAULT '{}'::text[] NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    embedding vector(1024),
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
    CONSTRAINT agent_memories_scope_check CHECK (scope IN ('org', 'user')),
    CONSTRAINT agent_memories_scope_user_check CHECK (
        (scope = 'org' AND user_id IS NULL) OR
        (scope = 'user' AND user_id IS NOT NULL)
    ),
    CONSTRAINT agent_memories_embedding_status_check CHECK (embedding_status IN ('pending', 'ready', 'failed'))
);

ALTER TABLE ONLY agent_memories ADD CONSTRAINT agent_memories_pkey PRIMARY KEY (id);

ALTER TABLE ONLY agent_memories
    ADD CONSTRAINT fk_agent_memories_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;
ALTER TABLE ONLY agent_memories
    ADD CONSTRAINT fk_agent_memories_agent FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE SET NULL;
ALTER TABLE ONLY agent_memories
    ADD CONSTRAINT fk_agent_memories_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE ONLY agent_memories
    ADD CONSTRAINT fk_agent_memories_source_session FOREIGN KEY (source_session_id) REFERENCES sessions(id) ON DELETE SET NULL;
ALTER TABLE ONLY agent_memories
    ADD CONSTRAINT fk_agent_memories_source_event FOREIGN KEY (source_event_id) REFERENCES session_events(id) ON DELETE SET NULL;
ALTER TABLE ONLY agent_memories
    ADD CONSTRAINT fk_agent_memories_created_by FOREIGN KEY (created_by_user_id) REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX idx_agent_memories_org_scope ON agent_memories USING btree (org_id, scope, created_at DESC)
    WHERE archived_at IS NULL;
CREATE INDEX idx_agent_memories_org_user ON agent_memories USING btree (org_id, user_id, created_at DESC)
    WHERE archived_at IS NULL AND scope = 'user';
CREATE INDEX idx_agent_memories_org_agent ON agent_memories USING btree (org_id, agent_id)
    WHERE archived_at IS NULL;
CREATE INDEX idx_agent_memories_embedding_status ON agent_memories USING btree (embedding_status, updated_at)
    WHERE archived_at IS NULL;
CREATE INDEX idx_agent_memories_tags ON agent_memories USING gin (tags)
    WHERE archived_at IS NULL;
CREATE INDEX idx_agent_memories_embedding_hnsw ON agent_memories
    USING hnsw (embedding vector_cosine_ops)
    WHERE archived_at IS NULL AND embedding_status = 'ready';

-- +goose Down
DROP TABLE IF EXISTS agent_memories;
