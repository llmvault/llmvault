-- +goose Up
CREATE TABLE public.api_keys (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    name text NOT NULL,
    key_hash text NOT NULL,
    key_prefix text NOT NULL,
    scopes text[] NOT NULL,
    expires_at timestamp with time zone,
    last_used_at timestamp with time zone,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone,
    created_by uuid
);

ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT api_keys_pkey PRIMARY KEY (id);

CREATE INDEX idx_api_keys_created_by ON public.api_keys USING btree (created_by);

CREATE UNIQUE INDEX idx_api_keys_key_hash ON public.api_keys USING btree (key_hash);

CREATE INDEX idx_api_keys_org_id ON public.api_keys USING btree (org_id);

ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT api_keys_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT fk_api_keys_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;
