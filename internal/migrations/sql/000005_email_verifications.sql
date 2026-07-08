-- +goose Up
CREATE TABLE public.email_verifications (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    token_hash text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    used_at timestamp with time zone,
    created_at timestamp with time zone
);

ALTER TABLE ONLY public.email_verifications
    ADD CONSTRAINT email_verifications_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX idx_email_verifications_token_hash ON public.email_verifications USING btree (token_hash);

CREATE INDEX idx_email_verifications_user_id ON public.email_verifications USING btree (user_id);
