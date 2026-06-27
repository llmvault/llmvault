-- +goose Up
CREATE TABLE slack_thread_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL PRIMARY KEY,
    org_id uuid NOT NULL,
    connection_id uuid NOT NULL,
    channel_id uuid,
    session_id uuid,
    session_event_id uuid,
    session_message_queue_id uuid,
    team_id text NOT NULL DEFAULT '',
    slack_channel_id text NOT NULL,
    thread_ts text NOT NULL,
    message_ts text NOT NULL,
    message_at timestamptz NOT NULL,
    event_id text NOT NULL DEFAULT '',
    event_type text NOT NULL,
    direction text NOT NULL,
    sender_id text NOT NULL DEFAULT '',
    text text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'received',
    ignore_reason text NOT NULL DEFAULT '',
    slack_reply_ts text NOT NULL DEFAULT '',
    runtime_stream_id text NOT NULL DEFAULT '',
    runtime_turn_id text NOT NULL DEFAULT '',
    error text NOT NULL DEFAULT '',
    raw jsonb NOT NULL DEFAULT '{}'::jsonb,
    received_at timestamptz NOT NULL DEFAULT now(),
    status_set_at timestamptz,
    enqueued_at timestamptz,
    job_started_at timestamptz,
    channel_resolved_at timestamptz,
    session_resolved_at timestamptz,
    runtime_posted_at timestamptz,
    final_received_at timestamptz,
    slack_reply_sent_at timestamptz,
    completed_at timestamptz,
    failed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_slack_thread_events_connection_event
    ON slack_thread_events USING btree (connection_id, event_id)
    WHERE event_id <> '';

CREATE INDEX idx_slack_thread_events_thread_direction
    ON slack_thread_events USING btree
        (org_id, connection_id, slack_channel_id, thread_ts, direction, message_at DESC);

CREATE INDEX idx_slack_thread_events_session
    ON slack_thread_events USING btree (session_id);

ALTER TABLE ONLY slack_thread_events
    ADD CONSTRAINT fk_slack_thread_events_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY slack_thread_events
    ADD CONSTRAINT fk_slack_thread_events_connection FOREIGN KEY (connection_id) REFERENCES connections(id) ON DELETE CASCADE;

ALTER TABLE ONLY slack_thread_events
    ADD CONSTRAINT fk_slack_thread_events_channel FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE SET NULL;

ALTER TABLE ONLY slack_thread_events
    ADD CONSTRAINT fk_slack_thread_events_session FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE SET NULL;

ALTER TABLE ONLY slack_thread_events
    ADD CONSTRAINT fk_slack_thread_events_session_event FOREIGN KEY (session_event_id) REFERENCES session_events(id) ON DELETE SET NULL;

ALTER TABLE ONLY slack_thread_events
    ADD CONSTRAINT fk_slack_thread_events_session_queue FOREIGN KEY (session_message_queue_id) REFERENCES session_message_queue(id) ON DELETE SET NULL;

-- +goose Down
DROP TABLE IF EXISTS slack_thread_events;
