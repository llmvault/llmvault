-- +goose Up
CREATE TABLE public.oauth_exchange_tokens (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    token_hash text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    used_at timestamp with time zone,
    created_at timestamp with time zone
);

ALTER TABLE ONLY public.oauth_exchange_tokens
    ADD CONSTRAINT oauth_exchange_tokens_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX idx_oauth_exchange_tokens_token_hash ON public.oauth_exchange_tokens USING btree (token_hash);

CREATE INDEX idx_oauth_exchange_tokens_user_id ON public.oauth_exchange_tokens USING btree (user_id);

-- +goose Down
DROP TABLE IF EXISTS public.oauth_exchange_tokens CASCADE;
