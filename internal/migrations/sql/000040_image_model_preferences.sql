-- +goose Up
ALTER TABLE agents
    ADD COLUMN image_model text DEFAULT ''::text NOT NULL,
    ADD COLUMN vector_image_model text DEFAULT ''::text NOT NULL;

ALTER TABLE channels
    ADD COLUMN image_model text DEFAULT ''::text NOT NULL,
    ADD COLUMN vector_image_model text DEFAULT ''::text NOT NULL;

ALTER TABLE sessions
    ADD COLUMN image_model text DEFAULT ''::text NOT NULL,
    ADD COLUMN vector_image_model text DEFAULT ''::text NOT NULL;

-- +goose Down
ALTER TABLE sessions
    DROP COLUMN IF EXISTS vector_image_model,
    DROP COLUMN IF EXISTS image_model;

ALTER TABLE channels
    DROP COLUMN IF EXISTS vector_image_model,
    DROP COLUMN IF EXISTS image_model;

ALTER TABLE agents
    DROP COLUMN IF EXISTS vector_image_model,
    DROP COLUMN IF EXISTS image_model;
