-- +goose Up
-- +goose StatementBegin
ALTER TABLE channel_env_vars
    ADD COLUMN description text NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE channel_env_vars
    DROP COLUMN description;
-- +goose StatementEnd
