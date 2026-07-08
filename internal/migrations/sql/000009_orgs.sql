-- +goose Up
CREATE TABLE public.orgs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    rate_limit bigint DEFAULT 1000 NOT NULL,
    active boolean DEFAULT true NOT NULL,
    allowed_origins text[],
    plan_slug character varying(64) DEFAULT 'free'::character varying NOT NULL,
    byok boolean DEFAULT false NOT NULL,
    logo_url text DEFAULT ''::text NOT NULL,
    website character varying(500) DEFAULT ''::character varying NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    prompt_company text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    sandbox_exposed_ports integer[] DEFAULT '{3000,5173,8000,8080}'::integer[] NOT NULL
);

ALTER TABLE ONLY public.orgs
    ADD CONSTRAINT orgs_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX idx_orgs_name ON public.orgs USING btree (name);

-- +goose Down
DROP TABLE IF EXISTS public.orgs CASCADE;
