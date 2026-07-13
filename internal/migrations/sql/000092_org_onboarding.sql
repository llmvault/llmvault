-- +goose Up
ALTER TABLE public.orgs
    ADD COLUMN onboarding_step text DEFAULT 'complete'::text NOT NULL;

ALTER TABLE public.orgs
    ADD CONSTRAINT orgs_onboarding_step_check
    CHECK (onboarding_step IN ('team', 'connections', 'welcome', 'complete'));

-- +goose Down
ALTER TABLE public.orgs
    DROP CONSTRAINT IF EXISTS orgs_onboarding_step_check;

ALTER TABLE public.orgs
    DROP COLUMN IF EXISTS onboarding_step;
