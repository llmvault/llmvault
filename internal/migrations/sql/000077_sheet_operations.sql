-- +goose Up
CREATE TABLE public.sheet_operations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    page_id uuid NOT NULL,
    type text NOT NULL,
    row_count integer DEFAULT 0 NOT NULL,
    inverse jsonb DEFAULT '{}'::jsonb NOT NULL,
    actor_agent_id uuid,
    actor_user_id uuid,
    source_session_id uuid,
    reverted_at timestamp with time zone,
    reverted_by_agent_id uuid,
    reverted_by_user_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT sheet_operations_type_check CHECK ((type = ANY (ARRAY['rows_insert'::text, 'rows_update'::text, 'rows_delete'::text, 'csv_import'::text, 'field_change'::text])))
);

ALTER TABLE ONLY public.sheet_operations
    ADD CONSTRAINT sheet_operations_pkey PRIMARY KEY (id);

CREATE INDEX idx_sheet_operations_org ON public.sheet_operations USING btree (org_id);

CREATE INDEX idx_sheet_operations_page_created ON public.sheet_operations USING btree (page_id, created_at DESC);

ALTER TABLE ONLY public.sheet_operations
    ADD CONSTRAINT sheet_operations_actor_agent_id_fkey FOREIGN KEY (actor_agent_id) REFERENCES public.agents(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.sheet_operations
    ADD CONSTRAINT sheet_operations_actor_user_id_fkey FOREIGN KEY (actor_user_id) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.sheet_operations
    ADD CONSTRAINT sheet_operations_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sheet_operations
    ADD CONSTRAINT sheet_operations_page_id_fkey FOREIGN KEY (page_id) REFERENCES public.sheet_pages(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sheet_operations
    ADD CONSTRAINT sheet_operations_reverted_by_agent_id_fkey FOREIGN KEY (reverted_by_agent_id) REFERENCES public.agents(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.sheet_operations
    ADD CONSTRAINT sheet_operations_reverted_by_user_id_fkey FOREIGN KEY (reverted_by_user_id) REFERENCES public.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY public.sheet_operations
    ADD CONSTRAINT sheet_operations_source_session_id_fkey FOREIGN KEY (source_session_id) REFERENCES public.sessions(id) ON DELETE SET NULL;
