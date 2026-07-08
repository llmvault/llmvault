-- +goose Up
CREATE TABLE public.team_plugins (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    team_id uuid NOT NULL,
    plugin_id uuid NOT NULL,
    enabled_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.team_plugins
    ADD CONSTRAINT team_plugins_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.team_plugins
    ADD CONSTRAINT team_plugins_team_plugin_unique UNIQUE (team_id, plugin_id);

CREATE INDEX idx_team_plugins_org_team ON public.team_plugins USING btree (org_id, team_id);

ALTER TABLE ONLY public.team_plugins
    ADD CONSTRAINT team_plugins_enabled_by_fkey FOREIGN KEY (enabled_by) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.team_plugins
    ADD CONSTRAINT team_plugins_plugin_id_fkey FOREIGN KEY (plugin_id) REFERENCES public.plugins(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.team_plugins
    ADD CONSTRAINT team_plugins_team_id_fkey FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;

-- +goose Down
DROP TABLE IF EXISTS public.team_plugins CASCADE;
