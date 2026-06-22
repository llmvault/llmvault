-- +goose Up
ALTER TABLE session_events
    ADD COLUMN IF NOT EXISTS runtime_seq bigint,
    ADD COLUMN IF NOT EXISTS runtime_event_id text,
    ADD COLUMN IF NOT EXISTS turn_id text,
    ADD COLUMN IF NOT EXISTS span_id text,
    ADD COLUMN IF NOT EXISTS durability text;

CREATE UNIQUE INDEX IF NOT EXISTS idx_session_events_runtime_sequence_number
    ON session_events (session_id, sequence_number)
    WHERE runtime_seq IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_session_events_runtime_seq
    ON session_events (session_id, runtime_seq)
    WHERE runtime_seq IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_session_events_runtime_event_id
    ON session_events (session_id, runtime_event_id)
    WHERE runtime_event_id IS NOT NULL AND runtime_event_id <> '';

CREATE INDEX IF NOT EXISTS idx_session_events_session_sequence
    ON session_events (session_id, sequence_number)
    WHERE sequence_number IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_session_events_session_sequence;
DROP INDEX IF EXISTS idx_session_events_runtime_event_id;
DROP INDEX IF EXISTS idx_session_events_runtime_seq;
DROP INDEX IF EXISTS idx_session_events_runtime_sequence_number;

ALTER TABLE session_events
    DROP COLUMN IF EXISTS durability,
    DROP COLUMN IF EXISTS span_id,
    DROP COLUMN IF EXISTS turn_id,
    DROP COLUMN IF EXISTS runtime_event_id,
    DROP COLUMN IF EXISTS runtime_seq;
