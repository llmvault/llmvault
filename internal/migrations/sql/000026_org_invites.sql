-- +goose Up
CREATE TABLE public.org_invites (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    email text NOT NULL,
    role text NOT NULL,
    token_hash text NOT NULL,
    invited_by_id uuid NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    accepted_at timestamp with time zone,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY public.org_invites
    ADD CONSTRAINT org_invites_pkey PRIMARY KEY (id);

CREATE INDEX idx_org_invites_email ON public.org_invites USING btree (email);

CREATE INDEX idx_org_invites_org_id ON public.org_invites USING btree (org_id);

CREATE UNIQUE INDEX idx_org_invites_token_hash ON public.org_invites USING btree (token_hash);

ALTER TABLE ONLY public.org_invites
    ADD CONSTRAINT fk_org_invites_invited_by FOREIGN KEY (invited_by_id) REFERENCES public.users(id);

ALTER TABLE ONLY public.org_invites
    ADD CONSTRAINT fk_org_invites_org FOREIGN KEY (org_id) REFERENCES public.orgs(id);

-- +goose Down
DROP TABLE IF EXISTS public.org_invites CASCADE;
