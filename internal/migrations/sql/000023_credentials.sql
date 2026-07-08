-- +goose Up
CREATE TABLE public.credentials (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid,
    label text DEFAULT ''::text NOT NULL,
    base_url text NOT NULL,
    auth_scheme text NOT NULL,
    encrypted_key bytea NOT NULL,
    wrapped_dek bytea NOT NULL,
    remaining bigint,
    refill_amount bigint,
    refill_interval text,
    last_refill_at timestamp with time zone,
    provider_id text DEFAULT ''::text,
    meta jsonb DEFAULT '{}'::jsonb,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone
);

ALTER TABLE ONLY public.credentials
    ADD CONSTRAINT credentials_pkey PRIMARY KEY (id);

CREATE INDEX idx_credentials_org_id ON public.credentials USING btree (org_id);

ALTER TABLE ONLY public.credentials
    ADD CONSTRAINT fk_credentials_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

-- +goose Down
DROP TABLE IF EXISTS public.credentials CASCADE;
