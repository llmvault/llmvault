-- +goose Up
CREATE TABLE public.slack_thread_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    connection_id uuid NOT NULL,
    channel_id uuid,
    session_id uuid,
    session_event_id uuid,
    session_message_queue_id uuid,
    team_id text DEFAULT ''::text NOT NULL,
    slack_channel_id text NOT NULL,
    thread_ts text NOT NULL,
    message_ts text NOT NULL,
    message_at timestamp with time zone NOT NULL,
    event_id text DEFAULT ''::text NOT NULL,
    event_type text NOT NULL,
    direction text NOT NULL,
    sender_id text DEFAULT ''::text NOT NULL,
    text text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'received'::text NOT NULL,
    ignore_reason text DEFAULT ''::text NOT NULL,
    slack_reply_ts text DEFAULT ''::text NOT NULL,
    runtime_stream_id text DEFAULT ''::text NOT NULL,
    runtime_turn_id text DEFAULT ''::text NOT NULL,
    error text DEFAULT ''::text NOT NULL,
    raw jsonb DEFAULT '{}'::jsonb NOT NULL,
    received_at timestamp with time zone DEFAULT now() NOT NULL,
    status_set_at timestamp with time zone,
    enqueued_at timestamp with time zone,
    job_started_at timestamp with time zone,
    channel_resolved_at timestamp with time zone,
    session_resolved_at timestamp with time zone,
    runtime_posted_at timestamp with time zone,
    final_received_at timestamp with time zone,
    slack_reply_sent_at timestamp with time zone,
    completed_at timestamp with time zone,
    failed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    trigger_id uuid
);

ALTER TABLE ONLY public.slack_thread_events
    ADD CONSTRAINT slack_thread_events_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX idx_slack_thread_events_connection_event ON public.slack_thread_events USING btree (connection_id, event_id) WHERE (event_id <> ''::text);

CREATE INDEX idx_slack_thread_events_session ON public.slack_thread_events USING btree (session_id);

CREATE INDEX idx_slack_thread_events_thread_direction ON public.slack_thread_events USING btree (org_id, connection_id, slack_channel_id, thread_ts, direction, message_at DESC);

CREATE INDEX idx_slack_thread_events_trigger_id ON public.slack_thread_events USING btree (trigger_id);

ALTER TABLE ONLY public.slack_thread_events
    ADD CONSTRAINT fk_slack_thread_events_channel FOREIGN KEY (channel_id) REFERENCES public.channels(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.slack_thread_events
    ADD CONSTRAINT fk_slack_thread_events_connection FOREIGN KEY (connection_id) REFERENCES public.connections(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.slack_thread_events
    ADD CONSTRAINT fk_slack_thread_events_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.slack_thread_events
    ADD CONSTRAINT fk_slack_thread_events_session FOREIGN KEY (session_id) REFERENCES public.sessions(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.slack_thread_events
    ADD CONSTRAINT fk_slack_thread_events_session_event FOREIGN KEY (session_event_id) REFERENCES public.session_events(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.slack_thread_events
    ADD CONSTRAINT fk_slack_thread_events_session_queue FOREIGN KEY (session_message_queue_id) REFERENCES public.session_message_queue(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.slack_thread_events
    ADD CONSTRAINT fk_slack_thread_events_trigger FOREIGN KEY (trigger_id) REFERENCES public.agent_triggers(id) ON DELETE SET NULL;
