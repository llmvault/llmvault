-- +goose Up
CREATE TABLE public.org_memberships (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    org_id uuid NOT NULL,
    role text DEFAULT 'owner'::text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY public.org_memberships
    ADD CONSTRAINT org_memberships_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX idx_membership_user_org ON public.org_memberships USING btree (user_id, org_id);

ALTER TABLE ONLY public.org_memberships
    ADD CONSTRAINT fk_org_memberships_org FOREIGN KEY (org_id) REFERENCES public.orgs(id);

ALTER TABLE ONLY public.org_memberships
    ADD CONSTRAINT fk_org_memberships_user FOREIGN KEY (user_id) REFERENCES public.users(id);

-- +goose Down
DROP TABLE IF EXISTS public.org_memberships CASCADE;
