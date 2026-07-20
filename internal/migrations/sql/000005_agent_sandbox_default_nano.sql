-- +goose Up

-- New agents default to the measured baseline tier. Existing agents retain
-- their explicitly stored size so this migration does not silently reduce an
-- existing customer's sandbox resources.
ALTER TABLE public.agents
    ALTER COLUMN sandbox_size SET DEFAULT 'nano'::text;

-- +goose Down

ALTER TABLE public.agents
    ALTER COLUMN sandbox_size SET DEFAULT 'small'::text;
