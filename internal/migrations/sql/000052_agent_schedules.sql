-- +goose Up
CREATE TABLE public.agent_schedules (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    sandbox_id uuid,
    runtime_job_id character varying(255) NOT NULL,
    status character varying(64) DEFAULT 'active'::character varying NOT NULL,
    channel character varying(255) DEFAULT ''::character varying NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    task_prompt text DEFAULT ''::text NOT NULL,
    interval_seconds bigint,
    repeat_count bigint,
    repeat_completed bigint DEFAULT 0 NOT NULL,
    next_run_at timestamp with time zone,
    last_run_at timestamp with time zone,
    last_status character varying(64) DEFAULT ''::character varying NOT NULL,
    last_error text DEFAULT ''::text NOT NULL,
    created_by_session character varying(255) DEFAULT ''::character varying NOT NULL,
    runtime_created_at timestamp with time zone,
    cancelled_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    is_system boolean DEFAULT false NOT NULL,
    provider text DEFAULT ''::text NOT NULL,
    connection_id uuid,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    schedule_kind text DEFAULT 'interval'::text NOT NULL,
    cron_expression text,
    lease_owner text DEFAULT ''::text NOT NULL,
    leased_until timestamp with time zone,
    source_slug text DEFAULT ''::text NOT NULL,
    name text
);

ALTER TABLE ONLY public.agent_schedules
    ADD CONSTRAINT agent_schedules_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX idx_agent_schedule_agent_runtime ON public.agent_schedules USING btree (agent_id, runtime_job_id);

CREATE UNIQUE INDEX idx_agent_schedules_agent_source_active ON public.agent_schedules USING btree (agent_id, source_slug) WHERE ((source_slug <> ''::text) AND (cancelled_at IS NULL) AND ((status)::text <> 'cancelled'::text));

CREATE INDEX idx_agent_schedules_cancelled_at ON public.agent_schedules USING btree (cancelled_at);

CREATE INDEX idx_agent_schedules_connection_id ON public.agent_schedules USING btree (connection_id);

CREATE INDEX idx_agent_schedules_due ON public.agent_schedules USING btree (status, next_run_at) WHERE (cancelled_at IS NULL);

CREATE INDEX idx_agent_schedules_is_system ON public.agent_schedules USING btree (is_system);

CREATE INDEX idx_agent_schedules_lease ON public.agent_schedules USING btree (leased_until) WHERE (lease_owner <> ''::text);

CREATE INDEX idx_agent_schedules_next_run_at ON public.agent_schedules USING btree (next_run_at);

CREATE INDEX idx_agent_schedules_org_id ON public.agent_schedules USING btree (org_id);

CREATE INDEX idx_agent_schedules_provider ON public.agent_schedules USING btree (provider);

CREATE INDEX idx_agent_schedules_sandbox_id ON public.agent_schedules USING btree (sandbox_id);

CREATE INDEX idx_agent_schedules_status ON public.agent_schedules USING btree (status);

ALTER TABLE ONLY public.agent_schedules
    ADD CONSTRAINT fk_agent_schedules_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_schedules
    ADD CONSTRAINT fk_agent_schedules_connection FOREIGN KEY (connection_id) REFERENCES public.connections(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.agent_schedules
    ADD CONSTRAINT fk_agent_schedules_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_schedules
    ADD CONSTRAINT fk_agent_schedules_sandbox FOREIGN KEY (sandbox_id) REFERENCES public.sandboxes(id) ON DELETE SET NULL;
