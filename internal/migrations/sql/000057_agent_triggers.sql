-- +goose Up
CREATE TABLE public.agent_triggers (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    trigger_type character varying(32) DEFAULT 'webhook'::character varying NOT NULL,
    connection_id uuid,
    trigger_keys text[] DEFAULT '{}'::text[] NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    conditions jsonb,
    instructions text DEFAULT ''::text NOT NULL,
    secret_key text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    source_slug text DEFAULT ''::text NOT NULL,
    channel_id uuid,
    trigger_key text DEFAULT ''::text NOT NULL,
    trigger_value text DEFAULT ''::text NOT NULL,
    name text
);

ALTER TABLE ONLY public.agent_triggers
    ADD CONSTRAINT agent_triggers_pkey PRIMARY KEY (id);

CREATE INDEX idx_agent_triggers_agent_id ON public.agent_triggers USING btree (agent_id);

CREATE UNIQUE INDEX idx_agent_triggers_agent_source_active ON public.agent_triggers USING btree (agent_id, source_slug) WHERE ((source_slug <> ''::text) AND (enabled = true));

CREATE INDEX idx_agent_triggers_channel_id ON public.agent_triggers USING btree (channel_id);

CREATE INDEX idx_agent_triggers_connection_id ON public.agent_triggers USING btree (connection_id);

CREATE UNIQUE INDEX idx_agent_triggers_enabled_key_value ON public.agent_triggers USING btree (org_id, connection_id, channel_id, trigger_key, trigger_value) WHERE ((enabled = true) AND (trigger_key <> ''::text) AND (trigger_value <> ''::text));

CREATE INDEX idx_agent_triggers_org_id ON public.agent_triggers USING btree (org_id);

CREATE INDEX idx_agent_triggers_trigger_key ON public.agent_triggers USING btree (trigger_key);

CREATE INDEX idx_agent_triggers_trigger_type ON public.agent_triggers USING btree (trigger_type);

ALTER TABLE ONLY public.agent_triggers
    ADD CONSTRAINT fk_agent_triggers_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_triggers
    ADD CONSTRAINT fk_agent_triggers_channel FOREIGN KEY (channel_id) REFERENCES public.channels(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.agent_triggers
    ADD CONSTRAINT fk_agent_triggers_connection FOREIGN KEY (connection_id) REFERENCES public.connections(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_triggers
    ADD CONSTRAINT fk_agent_triggers_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;
