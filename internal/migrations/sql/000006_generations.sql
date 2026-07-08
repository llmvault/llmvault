-- +goose Up
CREATE TABLE public.generations (
    id text NOT NULL,
    org_id uuid NOT NULL,
    credential_id uuid NOT NULL,
    token_jti text NOT NULL,
    provider_id text NOT NULL,
    model text,
    request_path text,
    is_streaming boolean DEFAULT false,
    input_tokens bigint DEFAULT 0,
    output_tokens bigint DEFAULT 0,
    cached_tokens bigint DEFAULT 0,
    reasoning_tokens bigint DEFAULT 0,
    cost numeric(12,8) DEFAULT 0,
    ttfb_ms bigint,
    total_ms bigint,
    upstream_status bigint,
    user_id text,
    tags text[],
    error_type text,
    error_message text,
    ip_address inet,
    created_at timestamp with time zone NOT NULL,
    is_system boolean DEFAULT false NOT NULL,
    billed_at timestamp with time zone,
    billing_error text,
    credits_debited bigint DEFAULT 0 NOT NULL,
    billing_cost_source text DEFAULT ''::text NOT NULL,
    billing_attempts integer DEFAULT 0 NOT NULL
);

ALTER TABLE ONLY public.generations
    ADD CONSTRAINT generations_pkey PRIMARY KEY (id);

CREATE INDEX idx_gen_billed_org_cost ON public.generations USING btree (org_id, cost) WHERE ((is_system = true) AND (billed_at IS NOT NULL) AND (billing_error = ''::text) AND (cost > (0)::numeric));

CREATE INDEX idx_gen_org_created ON public.generations USING btree (org_id, created_at);

CREATE INDEX idx_gen_org_credential ON public.generations USING btree (credential_id);

CREATE INDEX idx_gen_org_model ON public.generations USING btree (model);

CREATE INDEX idx_gen_org_provider ON public.generations USING btree (provider_id);

CREATE INDEX idx_gen_org_user ON public.generations USING btree (user_id);

CREATE INDEX idx_gen_unbilled_system_created ON public.generations USING btree (created_at) WHERE ((billed_at IS NULL) AND (is_system = true));

-- +goose Down
DROP TABLE IF EXISTS public.generations CASCADE;
