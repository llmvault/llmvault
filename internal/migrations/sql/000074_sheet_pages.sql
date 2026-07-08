-- +goose Up
CREATE TABLE public.sheet_pages (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    sheet_id uuid NOT NULL,
    org_id uuid NOT NULL,
    name text NOT NULL,
    "position" double precision NOT NULL,
    display_field_id text,
    archived_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.sheet_pages
    ADD CONSTRAINT sheet_pages_pkey PRIMARY KEY (id);

CREATE INDEX idx_sheet_pages_org ON public.sheet_pages USING btree (org_id);

CREATE UNIQUE INDEX idx_sheet_pages_sheet_name_active ON public.sheet_pages USING btree (sheet_id, name) WHERE (archived_at IS NULL);

ALTER TABLE ONLY public.sheet_pages
    ADD CONSTRAINT sheet_pages_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sheet_pages
    ADD CONSTRAINT sheet_pages_sheet_id_fkey FOREIGN KEY (sheet_id) REFERENCES public.sheets(id) ON DELETE CASCADE;
