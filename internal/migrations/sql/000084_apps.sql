-- +goose Up
CREATE TABLE public.apps (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    channel_id uuid NOT NULL,
    sheet_id uuid NOT NULL,
    slug text NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    icon text DEFAULT ''::text NOT NULL,
    alias text DEFAULT ''::text NOT NULL,
    sandbox_id uuid,
    encrypted_app_secret bytea NOT NULL,
    active_version_id uuid,
    status text DEFAULT 'draft'::text NOT NULL,
    template_version text DEFAULT ''::text NOT NULL,
    created_by_agent_id uuid,
    created_by_user_id uuid,
    source_session_id uuid,
    archived_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    preview_url text DEFAULT ''::text NOT NULL,
    preview_registered_at timestamp with time zone,
    alias_url text DEFAULT ''::text NOT NULL,
    CONSTRAINT apps_status_check CHECK ((status = ANY (ARRAY['draft'::text, 'deploying'::text, 'running'::text, 'stopped'::text, 'failed'::text])))
);

ALTER TABLE ONLY public.apps
    ADD CONSTRAINT apps_pkey PRIMARY KEY (id);

CREATE INDEX idx_apps_channel_updated_active ON public.apps USING btree (channel_id, updated_at DESC) WHERE (archived_at IS NULL);

CREATE UNIQUE INDEX idx_apps_org_slug_active ON public.apps USING btree (org_id, slug) WHERE (archived_at IS NULL);

CREATE INDEX idx_apps_sheet_active ON public.apps USING btree (sheet_id) WHERE (archived_at IS NULL);

ALTER TABLE ONLY public.apps
    ADD CONSTRAINT apps_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES public.channels(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.apps
    ADD CONSTRAINT apps_created_by_agent_id_fkey FOREIGN KEY (created_by_agent_id) REFERENCES public.agents(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.apps
    ADD CONSTRAINT apps_created_by_user_id_fkey FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.apps
    ADD CONSTRAINT apps_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.apps
    ADD CONSTRAINT apps_sandbox_id_fkey FOREIGN KEY (sandbox_id) REFERENCES public.sandboxes(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.apps
    ADD CONSTRAINT apps_sheet_id_fkey FOREIGN KEY (sheet_id) REFERENCES public.sheets(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.apps
    ADD CONSTRAINT apps_source_session_id_fkey FOREIGN KEY (source_session_id) REFERENCES public.sessions(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.apps
    ADD CONSTRAINT fk_apps_active_version FOREIGN KEY (active_version_id) REFERENCES public.app_versions(id) ON DELETE SET NULL;

-- +goose Down
DROP TABLE IF EXISTS public.apps CASCADE;
