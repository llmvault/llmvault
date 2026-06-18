-- +goose Up
ALTER TABLE skills
    ADD COLUMN IF NOT EXISTS human_description text;

-- +goose Down
ALTER TABLE skills
    DROP COLUMN IF EXISTS human_description;
