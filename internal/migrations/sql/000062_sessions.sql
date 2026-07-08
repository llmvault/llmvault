-- +goose Up
CREATE TABLE public.sessions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    channel_id uuid NOT NULL,
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
    vector_image_model text DEFAULT ''::text NOT NULL
);

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT sessions_pkey PRIMARY KEY (id);

CREATE INDEX idx_sessions_agent ON public.sessions USING btree (org_id, agent_id, created_at DESC);

CREATE INDEX idx_sessions_agent_turn_status ON public.sessions USING btree (agent_turn_status) WHERE (agent_turn_status <> 'idle'::text);

CREATE INDEX idx_sessions_channel ON public.sessions USING btree (channel_id, created_at DESC);

CREATE INDEX idx_sessions_sandbox_id ON public.sessions USING btree (sandbox_id);

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT fk_sessions_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE RESTRICT;

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT fk_sessions_channel FOREIGN KEY (channel_id) REFERENCES public.channels(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT fk_sessions_created_by FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT fk_sessions_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT fk_sessions_sandbox FOREIGN KEY (sandbox_id) REFERENCES public.sandboxes(id) ON DELETE SET NULL;
