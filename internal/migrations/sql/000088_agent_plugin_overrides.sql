-- +goose Up
CREATE TABLE public.agent_plugin_overrides (
    org_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    plugin_id uuid NOT NULL,
    disabled_by uuid,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    CONSTRAINT agent_plugin_overrides_pkey PRIMARY KEY (agent_id, plugin_id),
    CONSTRAINT fk_agent_plugin_overrides_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_plugin_overrides_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_plugin_overrides_plugin FOREIGN KEY (plugin_id) REFERENCES public.plugins(id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_plugin_overrides_disabled_by FOREIGN KEY (disabled_by) REFERENCES public.users(id) ON DELETE SET NULL
);

CREATE INDEX idx_agent_plugin_overrides_org_id ON public.agent_plugin_overrides USING btree (org_id);
CREATE INDEX idx_agent_plugin_overrides_plugin_id ON public.agent_plugin_overrides USING btree (plugin_id);
