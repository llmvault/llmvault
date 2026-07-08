-- +goose Up
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

ALTER TABLE ONLY public.session_events
    ADD CONSTRAINT session_events_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX idx_session_events_idem ON public.session_events USING btree (session_id, event_id) WHERE (event_id IS NOT NULL);

CREATE UNIQUE INDEX idx_session_events_runtime_event_id ON public.session_events USING btree (session_id, runtime_event_id) WHERE ((runtime_event_id IS NOT NULL) AND (runtime_event_id <> ''::text));

CREATE UNIQUE INDEX idx_session_events_runtime_seq ON public.session_events USING btree (session_id, runtime_seq) WHERE (runtime_seq IS NOT NULL);

CREATE UNIQUE INDEX idx_session_events_runtime_sequence_number ON public.session_events USING btree (session_id, sequence_number) WHERE (runtime_seq IS NOT NULL);

CREATE INDEX idx_session_events_session ON public.session_events USING btree (session_id, event_at);

CREATE INDEX idx_session_events_session_sequence ON public.session_events USING btree (session_id, sequence_number) WHERE (sequence_number IS NOT NULL);

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
