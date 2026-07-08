-- +goose Up
CREATE TABLE public.sheet_views (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    page_id uuid NOT NULL,
    org_id uuid NOT NULL,
    name text NOT NULL,
    type text DEFAULT 'grid'::text NOT NULL,
    config jsonb DEFAULT '{}'::jsonb NOT NULL,
    "position" double precision NOT NULL,
    archived_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.sheet_views
    ADD CONSTRAINT sheet_views_pkey PRIMARY KEY (id);

CREATE INDEX idx_sheet_views_org ON public.sheet_views USING btree (org_id);

CREATE INDEX idx_sheet_views_page_active ON public.sheet_views USING btree (page_id) WHERE (archived_at IS NULL);

ALTER TABLE ONLY public.sheet_views
    ADD CONSTRAINT sheet_views_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.sheet_views
    ADD CONSTRAINT sheet_views_page_id_fkey FOREIGN KEY (page_id) REFERENCES public.sheet_pages(id) ON DELETE CASCADE;

-- +goose Down
DROP TABLE IF EXISTS public.sheet_views CASCADE;
