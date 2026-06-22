-- +goose Up
ALTER TABLE session_message_queue
    ADD COLUMN IF NOT EXISTS actor_user_id uuid,
    ADD COLUMN IF NOT EXISTS message_text text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS message_payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS model text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS reasoning_effort text NOT NULL DEFAULT '';

ALTER TABLE session_message_queue
    ALTER COLUMN session_event_id DROP NOT NULL;

ALTER TABLE ONLY session_message_queue
    DROP CONSTRAINT IF EXISTS fk_session_message_queue_event;

ALTER TABLE ONLY session_message_queue
    ADD CONSTRAINT fk_session_message_queue_event FOREIGN KEY (session_event_id) REFERENCES session_events(id) ON DELETE SET NULL;

ALTER TABLE ONLY session_message_queue
    ADD CONSTRAINT fk_session_message_queue_actor_user FOREIGN KEY (actor_user_id) REFERENCES users(id) ON DELETE SET NULL;

DROP INDEX IF EXISTS idx_session_message_queue_session_event;
CREATE UNIQUE INDEX idx_session_message_queue_session_event
    ON session_message_queue USING btree (session_id, session_event_id)
    WHERE session_event_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_session_message_queue_session_event;
CREATE UNIQUE INDEX idx_session_message_queue_session_event
    ON session_message_queue USING btree (session_id, session_event_id);

ALTER TABLE ONLY session_message_queue
    DROP CONSTRAINT IF EXISTS fk_session_message_queue_actor_user;

ALTER TABLE ONLY session_message_queue
    DROP CONSTRAINT IF EXISTS fk_session_message_queue_event;

ALTER TABLE session_message_queue
    ALTER COLUMN session_event_id SET NOT NULL;

ALTER TABLE ONLY session_message_queue
    ADD CONSTRAINT fk_session_message_queue_event FOREIGN KEY (session_event_id) REFERENCES session_events(id) ON DELETE CASCADE;

ALTER TABLE session_message_queue
    DROP COLUMN IF EXISTS reasoning_effort,
    DROP COLUMN IF EXISTS model,
    DROP COLUMN IF EXISTS message_payload,
    DROP COLUMN IF EXISTS message_text,
    DROP COLUMN IF EXISTS actor_user_id;
