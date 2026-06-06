-- +goose Up
ALTER TABLE employee_schedules
    ADD COLUMN is_system boolean NOT NULL DEFAULT false,
    ADD COLUMN provider text NOT NULL DEFAULT '',
    ADD COLUMN connection_id uuid,
    ADD COLUMN metadata jsonb NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX idx_employee_schedules_is_system ON employee_schedules USING btree (is_system);
CREATE INDEX idx_employee_schedules_provider ON employee_schedules USING btree (provider);
CREATE INDEX idx_employee_schedules_connection_id ON employee_schedules USING btree (connection_id);

ALTER TABLE ONLY employee_schedules
    ADD CONSTRAINT fk_employee_schedules_connection FOREIGN KEY (connection_id) REFERENCES connections(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE ONLY employee_schedules
    DROP CONSTRAINT IF EXISTS fk_employee_schedules_connection;

DROP INDEX IF EXISTS idx_employee_schedules_connection_id;
DROP INDEX IF EXISTS idx_employee_schedules_provider;
DROP INDEX IF EXISTS idx_employee_schedules_is_system;

ALTER TABLE employee_schedules
    DROP COLUMN IF EXISTS metadata,
    DROP COLUMN IF EXISTS connection_id,
    DROP COLUMN IF EXISTS provider,
    DROP COLUMN IF EXISTS is_system;
