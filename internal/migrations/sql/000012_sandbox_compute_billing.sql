-- +goose Up

DROP TABLE IF EXISTS public.org_session_capacity_reservations;
DROP TABLE IF EXISTS public.rag_document_storage_usage;

ALTER TABLE public.orgs
    DROP CONSTRAINT IF EXISTS orgs_capacity_tier_check,
    DROP COLUMN IF EXISTS capacity_tier;

ALTER TABLE public.sandboxes
    ADD COLUMN vcpu integer NOT NULL DEFAULT 1 CHECK (vcpu > 0);

ALTER TABLE public.sessions
    ADD COLUMN sandbox_vcpu integer NOT NULL DEFAULT 1 CHECK (sandbox_vcpu > 0),
    ADD COLUMN sandbox_pricing_version integer NOT NULL DEFAULT 1,
    ADD COLUMN sandbox_credits_per_vcpu_minute integer NOT NULL DEFAULT 1 CHECK (sandbox_credits_per_vcpu_minute > 0);

CREATE TABLE public.sandbox_turn_usage (
    org_id uuid NOT NULL REFERENCES public.orgs(id) ON DELETE CASCADE,
    session_id uuid NOT NULL REFERENCES public.sessions(id) ON DELETE CASCADE,
    turn_id text NOT NULL,
    sandbox_vcpu integer NOT NULL CHECK (sandbox_vcpu > 0),
    pricing_version integer NOT NULL,
    credits_per_vcpu_minute integer NOT NULL CHECK (credits_per_vcpu_minute > 0),
    started_at timestamptz NOT NULL,
    observed_through timestamptz NOT NULL,
    ended_at timestamptz,
    active_milliseconds bigint NOT NULL DEFAULT 0 CHECK (active_milliseconds >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (session_id, turn_id)
);

CREATE INDEX idx_sandbox_turn_usage_org ON public.sandbox_turn_usage (org_id);
CREATE INDEX idx_sandbox_turn_usage_open ON public.sandbox_turn_usage (observed_through) WHERE ended_at IS NULL;
CREATE INDEX idx_session_events_sandbox_billing
    ON public.session_events (
        event_type,
        session_id,
        (COALESCE(NULLIF(turn_id, ''), payload->>'turn_id')),
        event_at
    )
    WHERE event_type IN ('turn_started', 'turn_completed', 'turn_failed', 'turn_interrupted');

-- +goose Down

DROP TABLE IF EXISTS public.sandbox_turn_usage;
DROP INDEX IF EXISTS public.idx_session_events_sandbox_billing;

ALTER TABLE public.sessions
    DROP COLUMN IF EXISTS sandbox_credits_per_vcpu_minute,
    DROP COLUMN IF EXISTS sandbox_pricing_version,
    DROP COLUMN IF EXISTS sandbox_vcpu;

ALTER TABLE public.sandboxes DROP COLUMN IF EXISTS vcpu;

ALTER TABLE public.orgs
    ADD COLUMN capacity_tier smallint DEFAULT 1 NOT NULL,
    ADD CONSTRAINT orgs_capacity_tier_check CHECK (capacity_tier >= 1 AND capacity_tier <= 4);

CREATE TABLE public.org_session_capacity_reservations (
    id uuid PRIMARY KEY,
    org_id uuid NOT NULL REFERENCES public.orgs(id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL
);

CREATE INDEX org_session_capacity_reservations_org_expiry
    ON public.org_session_capacity_reservations (org_id, expires_at);

CREATE TABLE public.rag_document_storage_usage (
    org_id uuid NOT NULL REFERENCES public.orgs(id) ON DELETE CASCADE,
    rag_source_id uuid NOT NULL REFERENCES public.rag_sources(id) ON DELETE CASCADE,
    document_id text NOT NULL,
    storage_bytes bigint NOT NULL CHECK (storage_bytes >= 0),
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    PRIMARY KEY (rag_source_id, document_id)
);
