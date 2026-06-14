-- +goose Up
-- Track whether a session is currently inside a runtime agent turn.

ALTER TABLE sessions ADD COLUMN IF NOT EXISTS agent_turn_status text DEFAULT 'idle'::text NOT NULL;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS agent_turn_id text DEFAULT ''::text NOT NULL;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS agent_stream_id text DEFAULT ''::text NOT NULL;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS agent_turn_started_at timestamp with time zone;

CREATE INDEX IF NOT EXISTS idx_sessions_agent_turn_status
    ON sessions USING btree (agent_turn_status)
    WHERE agent_turn_status <> 'idle';

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'baseline down migration is intentionally unsupported; reset or restore the database instead'; END $$;
-- +goose StatementEnd
