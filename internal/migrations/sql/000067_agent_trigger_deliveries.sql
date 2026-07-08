-- +goose Up
CREATE TABLE public.agent_trigger_deliveries (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    trigger_id uuid NOT NULL,
    connection_id uuid,
    delivery_id text NOT NULL,
    event_key text DEFAULT ''::text NOT NULL,
    resource_key text DEFAULT ''::text NOT NULL,
    session_id uuid NOT NULL,
    runtime_session_id text DEFAULT ''::text NOT NULL,
    runtime_stream_id text DEFAULT ''::text NOT NULL,
    runtime_trace_id text DEFAULT ''::text NOT NULL,
    runtime_turn_id text DEFAULT ''::text NOT NULL,
    payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone
);

ALTER TABLE ONLY public.agent_trigger_deliveries
    ADD CONSTRAINT agent_trigger_deliveries_pkey PRIMARY KEY (id);

CREATE INDEX idx_agent_trigger_deliveries_connection_id ON public.agent_trigger_deliveries USING btree (connection_id);

CREATE INDEX idx_agent_trigger_deliveries_delivery_id ON public.agent_trigger_deliveries USING btree (delivery_id);

CREATE INDEX idx_agent_trigger_deliveries_event_key ON public.agent_trigger_deliveries USING btree (event_key);

CREATE INDEX idx_agent_trigger_deliveries_resource_key ON public.agent_trigger_deliveries USING btree (resource_key);

CREATE INDEX idx_agent_trigger_deliveries_runtime_session_id ON public.agent_trigger_deliveries USING btree (runtime_session_id);

CREATE INDEX idx_agent_trigger_deliveries_session_id ON public.agent_trigger_deliveries USING btree (session_id);

CREATE UNIQUE INDEX idx_agent_trigger_deliveries_trigger_delivery ON public.agent_trigger_deliveries USING btree (trigger_id, delivery_id) WHERE (delivery_id <> ''::text);

CREATE INDEX idx_agent_trigger_deliveries_trigger_id ON public.agent_trigger_deliveries USING btree (trigger_id);

CREATE INDEX idx_trigger_delivery_org_agent_created ON public.agent_trigger_deliveries USING btree (org_id, agent_id, created_at);

CREATE INDEX idx_trigger_delivery_org_agent_session_created ON public.agent_trigger_deliveries USING btree (org_id, agent_id, runtime_session_id, created_at);

ALTER TABLE ONLY public.agent_trigger_deliveries
    ADD CONSTRAINT fk_agent_trigger_deliveries_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_trigger_deliveries
    ADD CONSTRAINT fk_agent_trigger_deliveries_connection FOREIGN KEY (connection_id) REFERENCES public.connections(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.agent_trigger_deliveries
    ADD CONSTRAINT fk_agent_trigger_deliveries_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_trigger_deliveries
    ADD CONSTRAINT fk_agent_trigger_deliveries_session FOREIGN KEY (session_id) REFERENCES public.sessions(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_trigger_deliveries
    ADD CONSTRAINT fk_agent_trigger_deliveries_trigger FOREIGN KEY (trigger_id) REFERENCES public.agent_triggers(id) ON DELETE CASCADE;

-- +goose Down
DROP TABLE IF EXISTS public.agent_trigger_deliveries CASCADE;
