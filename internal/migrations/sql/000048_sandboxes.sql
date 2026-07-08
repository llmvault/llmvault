-- +goose Up
CREATE TABLE public.sandboxes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid,
    agent_id uuid,
    sandbox_template_id uuid,
    snapshot_id text,
    provider_id text DEFAULT 'daytona'::text NOT NULL,
    external_id text NOT NULL,
    runtime_url text NOT NULL,
    runtime_url_expires_at timestamp with time zone,
    encrypted_runtime_secret bytea NOT NULL,
    status text DEFAULT 'creating'::text NOT NULL,
    error_message text,
    last_active_at timestamp with time zone,
    stopped_at timestamp with time zone,
    memory_limit_bytes bigint DEFAULT 0 NOT NULL,
    memory_used_bytes bigint DEFAULT 0 NOT NULL,
    memory_peak_bytes bigint DEFAULT 0 NOT NULL,
    cpu_quota text DEFAULT ''::text NOT NULL,
    cpu_usage_usec bigint DEFAULT 0 NOT NULL,
    cpu_throttled_count bigint DEFAULT 0 NOT NULL,
    pid_count bigint DEFAULT 0 NOT NULL,
    resource_checked_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    last_preview_at timestamp with time zone,
    exposed_ports integer[] DEFAULT '{3000,5173,8000,8080}'::integer[] NOT NULL,
    last_app_activity_at timestamp with time zone,
    last_gateway_activity_at timestamp with time zone
);

ALTER TABLE ONLY public.sandboxes
    ADD CONSTRAINT sandboxes_pkey PRIMARY KEY (id);

CREATE INDEX idx_sandboxes_agent_id ON public.sandboxes USING btree (agent_id);

CREATE INDEX idx_sandboxes_last_preview_at ON public.sandboxes USING btree (last_preview_at);

CREATE INDEX idx_sandboxes_org_id ON public.sandboxes USING btree (org_id);

ALTER TABLE ONLY public.sandboxes
    ADD CONSTRAINT fk_sandboxes_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sandboxes
    ADD CONSTRAINT fk_sandboxes_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sandboxes
    ADD CONSTRAINT fk_sandboxes_sandbox_template FOREIGN KEY (sandbox_template_id) REFERENCES public.sandbox_templates(id) ON DELETE SET NULL;

-- +goose Down
DROP TABLE IF EXISTS public.sandboxes CASCADE;
