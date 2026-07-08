-- +goose Up
CREATE TABLE public.otp_codes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    email text NOT NULL,
    token_hash text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    used_at timestamp with time zone,
    created_at timestamp with time zone
);

ALTER TABLE ONLY public.otp_codes
    ADD CONSTRAINT otp_codes_pkey PRIMARY KEY (id);

CREATE INDEX idx_otp_codes_email ON public.otp_codes USING btree (email);

CREATE UNIQUE INDEX idx_otp_codes_token_hash ON public.otp_codes USING btree (token_hash);
