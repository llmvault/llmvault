-- +goose Up
CREATE TABLE public.agent_directives (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    channel_id uuid,
    content text NOT NULL,
    created_by_user_id uuid,
    source text DEFAULT 'user-pinned'::text NOT NULL,
    active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    CONSTRAINT agent_directives_source_check CHECK ((source = ANY (ARRAY['user-pinned'::text, 'extracted-confirmed'::text])))
);

ALTER TABLE ONLY public.agent_directives
    ADD CONSTRAINT agent_directives_pkey PRIMARY KEY (id);

CREATE INDEX idx_agent_directives_org_channel ON public.agent_directives USING btree (org_id, channel_id) WHERE active;

ALTER TABLE ONLY public.agent_directives
    ADD CONSTRAINT agent_directives_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES public.channels(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_directives
    ADD CONSTRAINT agent_directives_created_by_user_id_fkey FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.agent_directives
    ADD CONSTRAINT agent_directives_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;
