-- +goose Up
CREATE TABLE public.sheet_fields (
    id text NOT NULL,
    page_id uuid NOT NULL,
    org_id uuid NOT NULL,
    name text NOT NULL,
    type text NOT NULL,
    options jsonb DEFAULT '{}'::jsonb NOT NULL,
    "position" double precision NOT NULL,
    archived_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.sheet_fields
    ADD CONSTRAINT sheet_fields_pkey PRIMARY KEY (id);

CREATE INDEX idx_sheet_fields_org ON public.sheet_fields USING btree (org_id);

CREATE INDEX idx_sheet_fields_page ON public.sheet_fields USING btree (page_id);

CREATE UNIQUE INDEX idx_sheet_fields_page_name_active ON public.sheet_fields USING btree (page_id, name) WHERE (archived_at IS NULL);

ALTER TABLE ONLY public.sheet_fields
    ADD CONSTRAINT sheet_fields_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sheet_fields
    ADD CONSTRAINT sheet_fields_page_id_fkey FOREIGN KEY (page_id) REFERENCES public.sheet_pages(id) ON DELETE CASCADE;

-- +goose Down
DROP TABLE IF EXISTS public.sheet_fields CASCADE;
