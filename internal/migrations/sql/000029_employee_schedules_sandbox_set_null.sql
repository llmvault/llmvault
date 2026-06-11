-- +goose Up
-- Employee cron schedules outlive the sandboxes they were last compiled for.
-- The original FK (migration 000011) was ON DELETE CASCADE, so a sandbox
-- hard-delete on any rollback/upgrade path cascade-wiped the org's schedules.
-- Repointing now happens explicitly after a successful main-runtime push; make
-- sandbox_id nullable and detach it on sandbox deletion instead of cascading.
ALTER TABLE ONLY employee_schedules
    DROP CONSTRAINT IF EXISTS fk_employee_schedules_sandbox;

ALTER TABLE employee_schedules
    ALTER COLUMN sandbox_id DROP NOT NULL;

ALTER TABLE ONLY employee_schedules
    ADD CONSTRAINT fk_employee_schedules_sandbox FOREIGN KEY (sandbox_id) REFERENCES sandboxes(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE ONLY employee_schedules
    DROP CONSTRAINT IF EXISTS fk_employee_schedules_sandbox;

ALTER TABLE employee_schedules
    ALTER COLUMN sandbox_id SET NOT NULL;

ALTER TABLE ONLY employee_schedules
    ADD CONSTRAINT fk_employee_schedules_sandbox FOREIGN KEY (sandbox_id) REFERENCES sandboxes(id) ON DELETE CASCADE;
