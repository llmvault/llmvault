-- +goose Up
CREATE TABLE public.password_resets (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    token_hash text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    used_at timestamp with time zone,
    created_at timestamp with time zone
);

ALTER TABLE ONLY public.password_resets
    ADD CONSTRAINT password_resets_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX idx_password_resets_token_hash ON public.password_resets USING btree (token_hash);

CREATE INDEX idx_password_resets_user_id ON public.password_resets USING btree (user_id);

-- +goose Down
DROP TABLE IF EXISTS public.password_resets CASCADE;
