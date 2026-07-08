-- +goose Up
CREATE TABLE public.agent_plugin_installs (
    org_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    plugin_id uuid NOT NULL,
    created_at timestamp with time zone
);

ALTER TABLE ONLY public.agent_plugin_installs
    ADD CONSTRAINT agent_plugin_installs_pkey PRIMARY KEY (agent_id, plugin_id);

CREATE INDEX idx_agent_plugin_installs_org_id ON public.agent_plugin_installs USING btree (org_id);

CREATE INDEX idx_agent_plugin_installs_plugin_id ON public.agent_plugin_installs USING btree (plugin_id);

ALTER TABLE ONLY public.agent_plugin_installs
    ADD CONSTRAINT fk_agent_plugin_installs_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_plugin_installs
    ADD CONSTRAINT fk_agent_plugin_installs_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_plugin_installs
    ADD CONSTRAINT fk_agent_plugin_installs_plugin FOREIGN KEY (plugin_id) REFERENCES public.plugins(id) ON DELETE CASCADE;

-- +goose Down
DROP TABLE IF EXISTS public.agent_plugin_installs CASCADE;
