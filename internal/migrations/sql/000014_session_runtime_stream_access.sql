-- +goose Up
-- Runtime stream metadata for browser-direct session SSE access.

ALTER TABLE session_message_queue
    ADD COLUMN IF NOT EXISTS runtime_stream_id text DEFAULT ''::text NOT NULL,
    ADD COLUMN IF NOT EXISTS runtime_stream_url text DEFAULT ''::text NOT NULL,
    ADD COLUMN IF NOT EXISTS runtime_trace_id text DEFAULT ''::text NOT NULL,
    ADD COLUMN IF NOT EXISTS runtime_turn_id text DEFAULT ''::text NOT NULL;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'baseline down migration is intentionally unsupported; reset or restore the database instead'; END $$;
-- +goose StatementEnd
