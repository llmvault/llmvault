-- +goose Up
CREATE TABLE public.plans (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    slug character varying(64) NOT NULL,
    name character varying(128) NOT NULL,
    provider character varying(32) DEFAULT ''::character varying NOT NULL,
    features jsonb,
    monthly_credits bigint DEFAULT 0 NOT NULL,
    welcome_credits bigint DEFAULT 0 NOT NULL,
    price_cents bigint DEFAULT 0 NOT NULL,
    currency character varying(8) DEFAULT 'USD'::character varying NOT NULL,
    active boolean DEFAULT true NOT NULL,
    visible boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY public.plans
    ADD CONSTRAINT plans_pkey PRIMARY KEY (id);

CREATE INDEX idx_plans_provider ON public.plans USING btree (provider);

CREATE UNIQUE INDEX idx_plans_slug ON public.plans USING btree (slug);

CREATE INDEX idx_plans_visible ON public.plans USING btree (visible);
