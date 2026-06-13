-- +goose Up
-- Gateway routes, events, and delivery bookkeeping

-- Provider-neutral agent gateway routes, inbound events, and outbound deliveries.

CREATE TABLE agent_gateway_routes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    connection_id uuid,
    provider character varying(128) NOT NULL,
    name text NOT NULL DEFAULT '',
    enabled boolean NOT NULL DEFAULT true,
    config jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    revoked_at timestamp with time zone
);

ALTER TABLE ONLY agent_gateway_routes
    ADD CONSTRAINT agent_gateway_routes_pkey PRIMARY KEY (id);

CREATE INDEX idx_agent_gateway_routes_org_agent ON agent_gateway_routes USING btree (org_id, agent_id);
CREATE INDEX idx_agent_gateway_routes_connection_id ON agent_gateway_routes USING btree (connection_id);
CREATE INDEX idx_agent_gateway_routes_provider ON agent_gateway_routes USING btree (provider);
CREATE INDEX idx_agent_gateway_routes_enabled ON agent_gateway_routes USING btree (org_id, enabled);

ALTER TABLE ONLY agent_gateway_routes
    ADD CONSTRAINT fk_agent_gateway_routes_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;
ALTER TABLE ONLY agent_gateway_routes
    ADD CONSTRAINT fk_agent_gateway_routes_agent FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE;
ALTER TABLE ONLY agent_gateway_routes
    ADD CONSTRAINT fk_agent_gateway_routes_connection FOREIGN KEY (connection_id) REFERENCES connections(id) ON DELETE SET NULL;

CREATE TABLE agent_gateway_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    route_id uuid NOT NULL,
    agent_session_id uuid,
    provider character varying(128) NOT NULL,
    external_message_id text NOT NULL DEFAULT '',
    dedupe_key text NOT NULL DEFAULT '',
    thread_key text NOT NULL DEFAULT '',
    channel_id text NOT NULL DEFAULT '',
    thread_id text NOT NULL DEFAULT '',
    sender_id text NOT NULL DEFAULT '',
    status character varying(32) NOT NULL DEFAULT 'received',
    error text NOT NULL DEFAULT '',
    runtime_conversation_id text NOT NULL DEFAULT '',
    runtime_session_id text NOT NULL DEFAULT '',
    runtime_stream_id text NOT NULL DEFAULT '',
    runtime_trace_id text NOT NULL DEFAULT '',
    runtime_turn_id text NOT NULL DEFAULT '',
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    received_at timestamp with time zone NOT NULL DEFAULT now(),
    processed_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY agent_gateway_events
    ADD CONSTRAINT agent_gateway_events_pkey PRIMARY KEY (id);

CREATE INDEX idx_agent_gateway_events_route_received ON agent_gateway_events USING btree (route_id, received_at);
CREATE INDEX idx_agent_gateway_events_org_agent_received ON agent_gateway_events USING btree (org_id, agent_id, received_at);
CREATE INDEX idx_agent_gateway_events_status_received ON agent_gateway_events USING btree (status, received_at);
CREATE INDEX idx_agent_gateway_events_session_id ON agent_gateway_events USING btree (agent_session_id);
CREATE UNIQUE INDEX idx_agent_gateway_events_route_dedupe ON agent_gateway_events USING btree (route_id, dedupe_key) WHERE dedupe_key <> '';

ALTER TABLE ONLY agent_gateway_events
    ADD CONSTRAINT fk_agent_gateway_events_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;
ALTER TABLE ONLY agent_gateway_events
    ADD CONSTRAINT fk_agent_gateway_events_agent FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE;
ALTER TABLE ONLY agent_gateway_events
    ADD CONSTRAINT fk_agent_gateway_events_route FOREIGN KEY (route_id) REFERENCES agent_gateway_routes(id) ON DELETE CASCADE;
ALTER TABLE ONLY agent_gateway_events
    ADD CONSTRAINT fk_agent_gateway_events_session FOREIGN KEY (agent_session_id) REFERENCES agent_sessions(id) ON DELETE SET NULL;

CREATE TABLE agent_gateway_deliveries (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    route_id uuid NOT NULL,
    agent_session_id uuid NOT NULL,
    provider character varying(128) NOT NULL,
    dedupe_key text NOT NULL DEFAULT '',
    runtime_session_id text NOT NULL DEFAULT '',
    runtime_trace_id text NOT NULL DEFAULT '',
    runtime_turn_id text NOT NULL DEFAULT '',
    thread_key text NOT NULL DEFAULT '',
    channel_id text NOT NULL DEFAULT '',
    thread_id text NOT NULL DEFAULT '',
    response_text text NOT NULL DEFAULT '',
    provider_handles jsonb NOT NULL DEFAULT '[]'::jsonb,
    status character varying(32) NOT NULL DEFAULT 'sent',
    error text NOT NULL DEFAULT '',
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY agent_gateway_deliveries
    ADD CONSTRAINT agent_gateway_deliveries_pkey PRIMARY KEY (id);

CREATE INDEX idx_agent_gateway_deliveries_route_created ON agent_gateway_deliveries USING btree (route_id, created_at);
CREATE INDEX idx_agent_gateway_deliveries_session_id ON agent_gateway_deliveries USING btree (agent_session_id);
CREATE UNIQUE INDEX idx_agent_gateway_deliveries_route_dedupe ON agent_gateway_deliveries USING btree (route_id, dedupe_key) WHERE dedupe_key <> '';

ALTER TABLE ONLY agent_gateway_deliveries
    ADD CONSTRAINT fk_agent_gateway_deliveries_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;
ALTER TABLE ONLY agent_gateway_deliveries
    ADD CONSTRAINT fk_agent_gateway_deliveries_agent FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE;
ALTER TABLE ONLY agent_gateway_deliveries
    ADD CONSTRAINT fk_agent_gateway_deliveries_route FOREIGN KEY (route_id) REFERENCES agent_gateway_routes(id) ON DELETE CASCADE;
ALTER TABLE ONLY agent_gateway_deliveries
    ADD CONSTRAINT fk_agent_gateway_deliveries_session FOREIGN KEY (agent_session_id) REFERENCES agent_sessions(id) ON DELETE CASCADE;

CREATE UNIQUE INDEX idx_agent_sessions_gateway_active_resource
    ON agent_sessions USING btree (org_id, agent_id, source, source_id, source_resource_key)
    WHERE status = 'active' AND source = 'gateway';

ALTER TABLE agent_gateway_events ALTER COLUMN route_id DROP NOT NULL;
ALTER TABLE agent_gateway_deliveries ALTER COLUMN route_id DROP NOT NULL;

-- Idempotency for gateway events/deliveries whose route_id is NULL.
--
-- Routes are resolved by connection at runtime and several paths create an
-- ephemeral route with ID == uuid.Nil (service.go / service_connection.go),
-- which persists as route_id IS NULL. The existing dedupe indexes
-- idx_agent_gateway_events_route_dedupe / _deliveries_route_dedupe are keyed
-- on (route_id, dedupe_key); Postgres treats NULLs as distinct, so the
-- ON CONFLICT DO NOTHING insert in gateway/store.go never fires for NULL-route
-- rows and a provider webhook redelivery (or an asynq retry of the stream
-- delivery) inserts a second row, re-driving the agent and double-posting the
-- reply. The recovery reads in store.go already key the NULL-route case on
-- (org_id, dedupe_key) (insertInboundEvent) / (dedupe_key) (loadDeliveryByDedupe),
-- so these partial indexes give that the unique enforcement it assumes.
--
-- The predicate mirrors each table's NULL-route read scope: events dedupe on
-- (org_id, dedupe_key) to match insertInboundEvent's recovery WHERE; deliveries
-- dedupe on dedupe_key alone to match loadDeliveryByDedupe / the pre-send dedupe
-- read in gateway_stream_delivery (both key NULL-route deliveries on dedupe_key
-- without org). dedupe_key <> '' keeps legacy/keyless rows out of the
-- idempotency scope, consistent with the existing route-scoped indexes.
CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_gateway_events_null_route_dedupe
    ON agent_gateway_events (org_id, dedupe_key)
    WHERE route_id IS NULL AND dedupe_key <> '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_gateway_deliveries_null_route_dedupe
    ON agent_gateway_deliveries (dedupe_key)
    WHERE route_id IS NULL AND dedupe_key <> '';

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'baseline down migration is intentionally unsupported; reset or restore the database instead'; END $$;
-- +goose StatementEnd
