-- +goose Up
CREATE TABLE public.session_message_queue (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    session_id uuid NOT NULL,
    session_event_id uuid,
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
    updated_at timestamp with time zone,
    actor_user_id uuid,
    message_text text DEFAULT ''::text NOT NULL,
    message_payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    model text DEFAULT ''::text NOT NULL,
    reasoning_effort text DEFAULT ''::text NOT NULL
);

ALTER TABLE ONLY public.session_message_queue
    ADD CONSTRAINT session_message_queue_pkey PRIMARY KEY (id);

CREATE INDEX idx_session_message_queue_claim ON public.session_message_queue USING btree (session_id, status, sequence_number);

CREATE UNIQUE INDEX idx_session_message_queue_sequence ON public.session_message_queue USING btree (session_id, sequence_number);

CREATE UNIQUE INDEX idx_session_message_queue_session_event ON public.session_message_queue USING btree (session_id, session_event_id) WHERE (session_event_id IS NOT NULL);

ALTER TABLE ONLY public.session_message_queue
    ADD CONSTRAINT fk_session_message_queue_actor_user FOREIGN KEY (actor_user_id) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.session_message_queue
    ADD CONSTRAINT fk_session_message_queue_event FOREIGN KEY (session_event_id) REFERENCES public.session_events(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.session_message_queue
    ADD CONSTRAINT fk_session_message_queue_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.session_message_queue
    ADD CONSTRAINT fk_session_message_queue_session FOREIGN KEY (session_id) REFERENCES public.sessions(id) ON DELETE CASCADE;

-- +goose Down
DROP TABLE IF EXISTS public.session_message_queue CASCADE;
