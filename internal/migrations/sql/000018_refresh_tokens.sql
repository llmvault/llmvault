-- +goose Up
CREATE TABLE public.refresh_tokens (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    token_hash text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone,
    replaced_at timestamp with time zone,
    replaced_by_access_token text,
    replaced_by_refresh_token text,
    created_at timestamp with time zone
);

ALTER TABLE ONLY public.refresh_tokens
    ADD CONSTRAINT refresh_tokens_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX idx_refresh_tokens_token_hash ON public.refresh_tokens USING btree (token_hash);

CREATE INDEX idx_refresh_tokens_user_id ON public.refresh_tokens USING btree (user_id);
