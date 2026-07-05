-- +goose Up
-- Optional human label for a schedule, shown in the automations UI. Nullable so
-- existing schedules are unaffected; the API populates it for new ones.
ALTER TABLE agent_schedules ADD COLUMN name text;

-- +goose Down
ALTER TABLE agent_schedules DROP COLUMN IF EXISTS name;
