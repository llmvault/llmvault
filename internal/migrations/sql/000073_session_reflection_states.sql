-- +goose Up
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

ALTER TABLE ONLY public.session_reflection_states
    ADD CONSTRAINT session_reflection_states_pkey PRIMARY KEY (session_id);

CREATE INDEX idx_session_reflection_states_scan ON public.session_reflection_states USING btree (status, locked_until, updated_at);

ALTER TABLE ONLY public.session_reflection_states
    ADD CONSTRAINT fk_session_reflection_states_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE RESTRICT;

ALTER TABLE ONLY public.session_reflection_states
    ADD CONSTRAINT fk_session_reflection_states_last_event FOREIGN KEY (last_reflected_event_id) REFERENCES public.session_events(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.session_reflection_states
    ADD CONSTRAINT fk_session_reflection_states_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.session_reflection_states
    ADD CONSTRAINT fk_session_reflection_states_session FOREIGN KEY (session_id) REFERENCES public.sessions(id) ON DELETE CASCADE;
