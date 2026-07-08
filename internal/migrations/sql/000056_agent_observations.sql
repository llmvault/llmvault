-- +goose Up
CREATE TABLE public.agent_observations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    channel_id uuid,
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

ALTER TABLE ONLY public.agent_observations
    ADD CONSTRAINT agent_observations_pkey PRIMARY KEY (id);

CREATE INDEX idx_agent_observations_embedding_hnsw ON public.agent_observations USING hnsw (embedding public.vector_cosine_ops) WHERE ((archived_at IS NULL) AND (embedding_status = 'ready'::text));

CREATE INDEX idx_agent_observations_expires ON public.agent_observations USING btree (expires_at) WHERE ((archived_at IS NULL) AND (expires_at IS NOT NULL));

CREATE INDEX idx_agent_observations_org_channel ON public.agent_observations USING btree (org_id, channel_id) WHERE (archived_at IS NULL);

ALTER TABLE ONLY public.agent_observations
    ADD CONSTRAINT fk_agent_observations_channel FOREIGN KEY (channel_id) REFERENCES public.channels(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_observations
    ADD CONSTRAINT fk_agent_observations_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_observations
    ADD CONSTRAINT fk_agent_observations_superseded_by FOREIGN KEY (superseded_by) REFERENCES public.agent_observations(id) ON DELETE SET NULL;
