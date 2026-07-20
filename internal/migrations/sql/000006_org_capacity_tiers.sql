-- +goose Up

ALTER TABLE public.orgs
    ADD COLUMN IF NOT EXISTS capacity_tier smallint DEFAULT 1 NOT NULL;

ALTER TABLE public.orgs
    DROP CONSTRAINT IF EXISTS orgs_capacity_tier_check;

ALTER TABLE public.orgs
    ADD CONSTRAINT orgs_capacity_tier_check
        CHECK (capacity_tier >= 1 AND capacity_tier <= 4);

-- Capacity unlocks are permanent. Seed existing organizations from the
-- lifetime USD-normalized credits granted by completed deposits.
WITH credited AS (
    SELECT org_id, COALESCE(SUM(credits), 0) AS credits
    FROM public.credit_purchases
    WHERE status = 'credited'
    GROUP BY org_id
)
UPDATE public.orgs AS org
SET capacity_tier = CASE
    WHEN credited.credits >= 500000 THEN 4
    WHEN credited.credits >= 250000 THEN 3
    WHEN credited.credits >= 100000 THEN 2
    ELSE 1
END
FROM credited
WHERE org.id = credited.org_id;

CREATE TABLE IF NOT EXISTS public.rag_document_storage_usage (
    org_id uuid NOT NULL,
    rag_source_id uuid NOT NULL,
    document_id text NOT NULL,
    storage_bytes bigint NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT rag_document_storage_usage_pkey PRIMARY KEY (rag_source_id, document_id),
    CONSTRAINT rag_document_storage_usage_bytes_check CHECK (storage_bytes >= 0),
    CONSTRAINT rag_document_storage_usage_org_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE,
    CONSTRAINT rag_document_storage_usage_source_fkey FOREIGN KEY (rag_source_id) REFERENCES public.rag_sources(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_rag_document_storage_usage_org_id
    ON public.rag_document_storage_usage (org_id);

CREATE TABLE IF NOT EXISTS public.org_session_capacity_reservations (
    id uuid PRIMARY KEY,
    org_id uuid NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT org_session_capacity_reservations_org_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_org_session_capacity_reservations_org_expiry
    ON public.org_session_capacity_reservations (org_id, expires_at);

-- +goose Down

DROP TABLE IF EXISTS public.org_session_capacity_reservations;

DROP TABLE IF EXISTS public.rag_document_storage_usage;

ALTER TABLE public.orgs
    DROP CONSTRAINT IF EXISTS orgs_capacity_tier_check,
    DROP COLUMN IF EXISTS capacity_tier;
