-- +goose Up
CREATE TABLE public.app_versions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    app_id uuid NOT NULL,
    org_id uuid NOT NULL,
    source_object_key text NOT NULL,
    bundle_object_key text NOT NULL,
    source_sha256 text NOT NULL,
    bundle_sha256 text NOT NULL,
    source_bytes bigint DEFAULT 0 NOT NULL,
    bundle_bytes bigint DEFAULT 0 NOT NULL,
    notes text DEFAULT ''::text NOT NULL,
    template_version text DEFAULT ''::text NOT NULL,
    created_by_agent_id uuid,
    source_session_id uuid,
    archived_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.app_versions
    ADD CONSTRAINT app_versions_pkey PRIMARY KEY (id);

CREATE INDEX idx_app_versions_app_created_active ON public.app_versions USING btree (app_id, created_at DESC) WHERE (archived_at IS NULL);

CREATE INDEX idx_app_versions_org ON public.app_versions USING btree (org_id);

ALTER TABLE ONLY public.app_versions
    ADD CONSTRAINT app_versions_created_by_agent_id_fkey FOREIGN KEY (created_by_agent_id) REFERENCES public.agents(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.app_versions
    ADD CONSTRAINT app_versions_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.app_versions
    ADD CONSTRAINT app_versions_source_session_id_fkey FOREIGN KEY (source_session_id) REFERENCES public.sessions(id) ON DELETE SET NULL;
