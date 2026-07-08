-- +goose Up
CREATE TABLE public.plugin_integrations (
    plugin_id uuid NOT NULL,
    provider text NOT NULL,
    kind text DEFAULT 'integration'::text NOT NULL,
    required boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone
);

ALTER TABLE ONLY public.plugin_integrations
    ADD CONSTRAINT plugin_integrations_pkey PRIMARY KEY (plugin_id, provider, kind);

ALTER TABLE ONLY public.plugin_integrations
    ADD CONSTRAINT fk_plugin_integrations_plugin FOREIGN KEY (plugin_id) REFERENCES public.plugins(id) ON DELETE CASCADE;
