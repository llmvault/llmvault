-- +goose Up
CREATE TABLE public.oauth_accounts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    provider text NOT NULL,
    provider_user_id text NOT NULL,
    provider_user_email text,
    provider_user_login text,
    verified_emails text[],
    last_synced_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY public.oauth_accounts
    ADD CONSTRAINT oauth_accounts_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX idx_oauth_provider_uid ON public.oauth_accounts USING btree (provider, provider_user_id);

CREATE UNIQUE INDEX idx_oauth_user_provider ON public.oauth_accounts USING btree (user_id, provider);

ALTER TABLE ONLY public.oauth_accounts
    ADD CONSTRAINT fk_oauth_accounts_user FOREIGN KEY (user_id) REFERENCES public.users(id);
