-- +goose Up
CREATE TABLE public.sheets (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    slug text NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    icon text DEFAULT ''::text NOT NULL,
    created_by_agent_id uuid,
    created_by_user_id uuid,
    source_session_id uuid,
    archived_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    channel_id uuid NOT NULL
);

ALTER TABLE ONLY public.sheets
    ADD CONSTRAINT sheets_pkey PRIMARY KEY (id);

CREATE INDEX idx_sheets_channel_updated_active ON public.sheets USING btree (channel_id, updated_at DESC) WHERE (archived_at IS NULL);

CREATE UNIQUE INDEX idx_sheets_org_slug_active ON public.sheets USING btree (org_id, slug) WHERE (archived_at IS NULL);

CREATE INDEX idx_sheets_org_updated_active ON public.sheets USING btree (org_id, updated_at DESC) WHERE (archived_at IS NULL);

ALTER TABLE ONLY public.sheets
    ADD CONSTRAINT sheets_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES public.channels(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sheets
    ADD CONSTRAINT sheets_created_by_agent_id_fkey FOREIGN KEY (created_by_agent_id) REFERENCES public.agents(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.sheets
    ADD CONSTRAINT sheets_created_by_user_id_fkey FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.sheets
    ADD CONSTRAINT sheets_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sheets
    ADD CONSTRAINT sheets_source_session_id_fkey FOREIGN KEY (source_session_id) REFERENCES public.sessions(id) ON DELETE SET NULL;
