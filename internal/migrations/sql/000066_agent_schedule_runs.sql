-- +goose Up
CREATE TABLE public.agent_schedule_runs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    schedule_id uuid NOT NULL,
    sandbox_id uuid,
    runtime_job_id character varying(255) NOT NULL,
    run_key character varying(500) NOT NULL,
    status character varying(64) DEFAULT 'running'::character varying NOT NULL,
    scheduled_at timestamp with time zone,
    started_at timestamp with time zone,
    completed_at timestamp with time zone,
    duration_ms bigint,
    error text DEFAULT ''::text NOT NULL,
    event_payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    session_id uuid,
    lease_owner text DEFAULT ''::text NOT NULL,
    leased_until timestamp with time zone
);

ALTER TABLE ONLY public.agent_schedule_runs
    ADD CONSTRAINT agent_schedule_runs_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX idx_agent_schedule_run_key ON public.agent_schedule_runs USING btree (schedule_id, run_key);

CREATE INDEX idx_agent_schedule_runs_agent_id ON public.agent_schedule_runs USING btree (agent_id);

CREATE INDEX idx_agent_schedule_runs_lease ON public.agent_schedule_runs USING btree (leased_until) WHERE (lease_owner <> ''::text);

CREATE INDEX idx_agent_schedule_runs_org_id ON public.agent_schedule_runs USING btree (org_id);

CREATE INDEX idx_agent_schedule_runs_runtime_job_id ON public.agent_schedule_runs USING btree (runtime_job_id);

CREATE INDEX idx_agent_schedule_runs_sandbox_id ON public.agent_schedule_runs USING btree (sandbox_id);

CREATE INDEX idx_agent_schedule_runs_scheduled_at ON public.agent_schedule_runs USING btree (scheduled_at);

CREATE INDEX idx_agent_schedule_runs_session_id ON public.agent_schedule_runs USING btree (session_id);

CREATE INDEX idx_agent_schedule_runs_status ON public.agent_schedule_runs USING btree (status);

ALTER TABLE ONLY public.agent_schedule_runs
    ADD CONSTRAINT fk_agent_schedule_runs_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_schedule_runs
    ADD CONSTRAINT fk_agent_schedule_runs_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_schedule_runs
    ADD CONSTRAINT fk_agent_schedule_runs_sandbox FOREIGN KEY (sandbox_id) REFERENCES public.sandboxes(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_schedule_runs
    ADD CONSTRAINT fk_agent_schedule_runs_schedule FOREIGN KEY (schedule_id) REFERENCES public.agent_schedules(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.agent_schedule_runs
    ADD CONSTRAINT fk_agent_schedule_runs_session FOREIGN KEY (session_id) REFERENCES public.sessions(id) ON DELETE SET NULL;

-- +goose Down
DROP TABLE IF EXISTS public.agent_schedule_runs CASCADE;
