-- +goose Up
CREATE TABLE public.sheet_import_jobs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    page_id uuid NOT NULL,
    object_key text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    total_rows bigint DEFAULT 0 NOT NULL,
    processed_rows bigint DEFAULT 0 NOT NULL,
    error text DEFAULT ''::text NOT NULL,
    options jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_by_agent_id uuid,
    created_by_user_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT sheet_import_jobs_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'running'::text, 'completed'::text, 'failed'::text])))
);

ALTER TABLE ONLY public.sheet_import_jobs
    ADD CONSTRAINT sheet_import_jobs_pkey PRIMARY KEY (id);

CREATE INDEX idx_sheet_import_jobs_org ON public.sheet_import_jobs USING btree (org_id);

CREATE INDEX idx_sheet_import_jobs_page ON public.sheet_import_jobs USING btree (page_id);

ALTER TABLE ONLY public.sheet_import_jobs
    ADD CONSTRAINT sheet_import_jobs_created_by_agent_id_fkey FOREIGN KEY (created_by_agent_id) REFERENCES public.agents(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.sheet_import_jobs
    ADD CONSTRAINT sheet_import_jobs_created_by_user_id_fkey FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.sheet_import_jobs
    ADD CONSTRAINT sheet_import_jobs_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sheet_import_jobs
    ADD CONSTRAINT sheet_import_jobs_page_id_fkey FOREIGN KEY (page_id) REFERENCES public.sheet_pages(id) ON DELETE CASCADE;
