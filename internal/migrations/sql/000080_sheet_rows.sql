-- +goose Up
CREATE TABLE public.sheet_rows (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    page_id uuid NOT NULL,
    org_id uuid NOT NULL,
    data jsonb DEFAULT '{}'::jsonb NOT NULL,
    "position" double precision NOT NULL,
    import_job_id uuid,
    created_by_agent_id uuid,
    created_by_user_id uuid,
    archived_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.sheet_rows
    ADD CONSTRAINT sheet_rows_pkey PRIMARY KEY (id);

CREATE INDEX idx_sheet_rows_data_gin ON public.sheet_rows USING gin (data jsonb_path_ops);

CREATE INDEX idx_sheet_rows_import_job ON public.sheet_rows USING btree (import_job_id) WHERE (import_job_id IS NOT NULL);

CREATE INDEX idx_sheet_rows_page_created ON public.sheet_rows USING btree (page_id, created_at, id);

CREATE INDEX idx_sheet_rows_page_position_active ON public.sheet_rows USING btree (page_id, "position") WHERE (archived_at IS NULL);

ALTER TABLE ONLY public.sheet_rows
    ADD CONSTRAINT sheet_rows_created_by_agent_id_fkey FOREIGN KEY (created_by_agent_id) REFERENCES public.agents(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.sheet_rows
    ADD CONSTRAINT sheet_rows_created_by_user_id_fkey FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.sheet_rows
    ADD CONSTRAINT sheet_rows_import_job_id_fkey FOREIGN KEY (import_job_id) REFERENCES public.sheet_import_jobs(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.sheet_rows
    ADD CONSTRAINT sheet_rows_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sheet_rows
    ADD CONSTRAINT sheet_rows_page_id_fkey FOREIGN KEY (page_id) REFERENCES public.sheet_pages(id) ON DELETE CASCADE;
