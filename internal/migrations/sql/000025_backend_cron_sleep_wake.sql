-- +goose Up
-- Backend-owned agent cron scheduling and sandbox preview activity.

ALTER TABLE sandboxes
    ADD COLUMN IF NOT EXISTS last_preview_at timestamp with time zone;

ALTER TABLE agent_schedules
    ADD COLUMN IF NOT EXISTS schedule_kind text NOT NULL DEFAULT 'interval',
    ADD COLUMN IF NOT EXISTS cron_expression text,
    ADD COLUMN IF NOT EXISTS lease_owner text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS leased_until timestamp with time zone;

ALTER TABLE agent_schedule_runs
    ADD COLUMN IF NOT EXISTS session_id uuid,
    ADD COLUMN IF NOT EXISTS lease_owner text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS leased_until timestamp with time zone;

CREATE INDEX IF NOT EXISTS idx_sandboxes_last_preview_at
    ON sandboxes USING btree (last_preview_at);

CREATE INDEX IF NOT EXISTS idx_agent_schedules_due
    ON agent_schedules USING btree (status, next_run_at)
    WHERE cancelled_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_agent_schedules_lease
    ON agent_schedules USING btree (leased_until)
    WHERE lease_owner <> '';

CREATE INDEX IF NOT EXISTS idx_agent_schedule_runs_session_id
    ON agent_schedule_runs USING btree (session_id);

CREATE INDEX IF NOT EXISTS idx_agent_schedule_runs_lease
    ON agent_schedule_runs USING btree (leased_until)
    WHERE lease_owner <> '';

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'fk_agent_schedule_runs_session'
    ) THEN
        ALTER TABLE ONLY agent_schedule_runs
            ADD CONSTRAINT fk_agent_schedule_runs_session
            FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE SET NULL;
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
ALTER TABLE ONLY agent_schedule_runs
    DROP CONSTRAINT IF EXISTS fk_agent_schedule_runs_session;

DROP INDEX IF EXISTS idx_agent_schedule_runs_lease;
DROP INDEX IF EXISTS idx_agent_schedule_runs_session_id;
DROP INDEX IF EXISTS idx_agent_schedules_lease;
DROP INDEX IF EXISTS idx_agent_schedules_due;
DROP INDEX IF EXISTS idx_sandboxes_last_preview_at;

ALTER TABLE agent_schedule_runs
    DROP COLUMN IF EXISTS leased_until,
    DROP COLUMN IF EXISTS lease_owner,
    DROP COLUMN IF EXISTS session_id;

ALTER TABLE agent_schedules
    DROP COLUMN IF EXISTS leased_until,
    DROP COLUMN IF EXISTS lease_owner,
    DROP COLUMN IF EXISTS cron_expression,
    DROP COLUMN IF EXISTS schedule_kind;

ALTER TABLE sandboxes
    DROP COLUMN IF EXISTS last_preview_at;
