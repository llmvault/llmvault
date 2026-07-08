-- +goose Up
CREATE TABLE public.tokens (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    credential_id uuid NOT NULL,
    jti text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    remaining bigint,
    refill_amount bigint,
    refill_interval text,
    last_refill_at timestamp with time zone,
    scopes jsonb,
    meta jsonb DEFAULT '{}'::jsonb,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone
);

ALTER TABLE ONLY public.tokens
    ADD CONSTRAINT tokens_pkey PRIMARY KEY (id);

CREATE INDEX idx_tokens_credential_id ON public.tokens USING btree (credential_id);

CREATE UNIQUE INDEX idx_tokens_jti ON public.tokens USING btree (jti);

ALTER TABLE ONLY public.tokens
    ADD CONSTRAINT fk_tokens_credential FOREIGN KEY (credential_id) REFERENCES public.credentials(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.tokens
    ADD CONSTRAINT fk_tokens_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

-- +goose Down
DROP TABLE IF EXISTS public.tokens CASCADE;
