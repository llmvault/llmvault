-- +goose Up
CREATE TABLE public.org_plugin_installs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    plugin_id uuid NOT NULL,
    created_by_user_id uuid,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY public.org_plugin_installs
    ADD CONSTRAINT org_plugin_installs_pkey PRIMARY KEY (id);

CREATE INDEX idx_org_plugin_installs_created_by_user_id ON public.org_plugin_installs USING btree (created_by_user_id);

CREATE UNIQUE INDEX idx_org_plugin_installs_one_active ON public.org_plugin_installs USING btree (org_id, plugin_id) WHERE (revoked_at IS NULL);

CREATE INDEX idx_org_plugin_installs_org_id ON public.org_plugin_installs USING btree (org_id);

CREATE INDEX idx_org_plugin_installs_plugin_id ON public.org_plugin_installs USING btree (plugin_id);

CREATE INDEX idx_org_plugin_installs_revoked_at ON public.org_plugin_installs USING btree (revoked_at);

ALTER TABLE ONLY public.org_plugin_installs
    ADD CONSTRAINT fk_org_plugin_installs_created_by FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.org_plugin_installs
    ADD CONSTRAINT fk_org_plugin_installs_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.org_plugin_installs
    ADD CONSTRAINT fk_org_plugin_installs_plugin FOREIGN KEY (plugin_id) REFERENCES public.plugins(id) ON DELETE CASCADE;
