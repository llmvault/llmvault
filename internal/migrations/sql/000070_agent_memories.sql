-- +goose Up
CREATE TABLE public.agent_memories (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
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
    channel_id uuid,
    consolidated_at timestamp with time zone,
    CONSTRAINT agent_memories_embedding_status_check CHECK ((embedding_status = ANY (ARRAY['pending'::text, 'ready'::text, 'failed'::text])))
);

ALTER TABLE ONLY public.agent_memories
    ADD CONSTRAINT agent_memories_pkey PRIMARY KEY (id);

CREATE INDEX idx_agent_memories_embedding_hnsw ON public.agent_memories USING hnsw (embedding public.vector_cosine_ops) WHERE ((archived_at IS NULL) AND (embedding_status = 'ready'::text));

CREATE INDEX idx_agent_memories_embedding_status ON public.agent_memories USING btree (embedding_status, updated_at) WHERE (archived_at IS NULL);

CREATE UNIQUE INDEX idx_agent_memories_fingerprint ON public.agent_memories USING btree (memory_fingerprint) WHERE ((archived_at IS NULL) AND (memory_fingerprint <> ''::text));

CREATE INDEX idx_agent_memories_org_channel ON public.agent_memories USING btree (org_id, channel_id, created_at DESC) WHERE (archived_at IS NULL);

CREATE INDEX idx_agent_memories_tags ON public.agent_memories USING gin (tags) WHERE (archived_at IS NULL);

CREATE INDEX idx_agent_memories_unconsolidated ON public.agent_memories USING btree (org_id, channel_id, created_at) WHERE ((archived_at IS NULL) AND (consolidated_at IS NULL));

ALTER TABLE ONLY public.agent_memories
    ADD CONSTRAINT fk_agent_memories_channel FOREIGN KEY (channel_id) REFERENCES public.channels(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_memories
    ADD CONSTRAINT fk_agent_memories_created_by FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.agent_memories
    ADD CONSTRAINT fk_agent_memories_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_memories
    ADD CONSTRAINT fk_agent_memories_source_event FOREIGN KEY (source_event_id) REFERENCES public.session_events(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.agent_memories
    ADD CONSTRAINT fk_agent_memories_source_session FOREIGN KEY (source_session_id) REFERENCES public.sessions(id) ON DELETE SET NULL;
