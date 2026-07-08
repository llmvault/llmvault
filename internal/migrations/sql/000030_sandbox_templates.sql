-- +goose Up
CREATE TABLE public.sandbox_templates (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    slug text NOT NULL,
    tags jsonb DEFAULT '[]'::jsonb NOT NULL,
    size text DEFAULT 'medium'::text NOT NULL,
    base_template_id uuid,
    build_commands text DEFAULT ''::text NOT NULL,
    provider_id text DEFAULT 'daytona'::text NOT NULL,
    external_id text,
    base_image_ref text,
    build_status text DEFAULT 'pending'::text NOT NULL,
    build_error text,
    build_logs text DEFAULT ''::text NOT NULL,
    config jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY public.sandbox_templates
    ADD CONSTRAINT sandbox_templates_pkey PRIMARY KEY (id);

CREATE INDEX idx_sandbox_templates_base_template_id ON public.sandbox_templates USING btree (base_template_id);

CREATE INDEX idx_sandbox_templates_org_id ON public.sandbox_templates USING btree (org_id);

CREATE UNIQUE INDEX idx_sandbox_templates_slug ON public.sandbox_templates USING btree (slug);

ALTER TABLE ONLY public.sandbox_templates
    ADD CONSTRAINT fk_sandbox_templates_base_template FOREIGN KEY (base_template_id) REFERENCES public.sandbox_templates(id);

ALTER TABLE ONLY public.sandbox_templates
    ADD CONSTRAINT fk_sandbox_templates_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE;
