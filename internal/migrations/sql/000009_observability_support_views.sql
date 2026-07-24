-- +goose Up

-- These views are the only application tables exposed to Grafana. They contain
-- operational identifiers and aggregates, but deliberately exclude customer
-- messages, prompts, email addresses/bodies, webhook payloads, credentials,
-- integration configuration, and raw error text.

CREATE OR REPLACE VIEW public.observability_session_support AS
SELECT
    s.id AS session_id,
    s.org_id,
    s.team_id,
    s.agent_id,
    s.sandbox_id,
    s.source,
    s.status,
    s.agent_turn_status,
    s.agent_turn_last_outcome,
    s.created_at,
    s.updated_at,
    s.ended_at,
    COALESCE(events.event_count, 0) AS event_count,
    COALESCE(messages.pending_message_count, 0) AS pending_message_count,
    COALESCE(messages.failed_message_count, 0) AS failed_message_count,
    COALESCE(generations.generation_count, 0) AS generation_count,
    COALESCE(generations.generation_error_count, 0) AS generation_error_count,
    COALESCE(generations.input_tokens, 0) AS input_tokens,
    COALESCE(generations.output_tokens, 0) AS output_tokens,
    COALESCE(generations.generation_cost, 0) AS generation_cost,
    COALESCE(generations.max_generation_ms, 0) AS max_generation_ms
FROM public.sessions s
LEFT JOIN LATERAL (
    SELECT COUNT(*) AS event_count
    FROM public.session_events se
    WHERE se.session_id = s.id
) events ON true
LEFT JOIN LATERAL (
    SELECT
        COUNT(*) FILTER (WHERE smq.status = 'pending') AS pending_message_count,
        COUNT(*) FILTER (WHERE smq.status IN ('failed', 'dead')) AS failed_message_count
    FROM public.session_message_queue smq
    WHERE smq.session_id = s.id
) messages ON true
LEFT JOIN LATERAL (
    SELECT
        COUNT(*) AS generation_count,
        COUNT(*) FILTER (WHERE g.error_type IS NOT NULL OR g.upstream_status >= 400) AS generation_error_count,
        SUM(g.input_tokens) AS input_tokens,
        SUM(g.output_tokens) AS output_tokens,
        SUM(g.cost) AS generation_cost,
        MAX(g.total_ms) AS max_generation_ms
    FROM public.generations g
    WHERE g.session_id = s.id
) generations ON true;

CREATE OR REPLACE VIEW public.observability_org_support AS
SELECT
    o.id AS org_id,
    o.active,
    o.byok,
    o.onboarding_step,
    o.created_at,
    COALESCE(agents.active_agent_count, 0) AS active_agent_count,
    COALESCE(sessions.session_count, 0) AS session_count,
    COALESCE(sessions.sessions_24h, 0) AS sessions_24h,
    COALESCE(sessions.failed_session_count, 0) AS failed_session_count,
    COALESCE(connections.active_connection_count, 0) AS active_connection_count,
    COALESCE(rag.enabled_rag_source_count, 0) AS enabled_rag_source_count,
    COALESCE(credits.credit_balance, 0) AS credit_balance,
    sessions.last_session_at
FROM public.orgs o
LEFT JOIN LATERAL (
    SELECT COUNT(*) FILTER (WHERE a.status = 'active') AS active_agent_count
    FROM public.agents a
    WHERE a.org_id = o.id
) agents ON true
LEFT JOIN LATERAL (
    SELECT
        COUNT(*) AS session_count,
        COUNT(*) FILTER (WHERE s.created_at >= now() - interval '24 hours') AS sessions_24h,
        COUNT(*) FILTER (
            WHERE s.agent_turn_last_outcome IN ('failed', 'error')
               OR s.agent_turn_status = 'failed'
        ) AS failed_session_count,
        MAX(s.created_at) AS last_session_at
    FROM public.sessions s
    WHERE s.org_id = o.id
) sessions ON true
LEFT JOIN LATERAL (
    SELECT COUNT(*) FILTER (WHERE c.revoked_at IS NULL) AS active_connection_count
    FROM public.connections c
    WHERE c.org_id = o.id
) connections ON true
LEFT JOIN LATERAL (
    SELECT COUNT(*) FILTER (WHERE rs.enabled) AS enabled_rag_source_count
    FROM public.rag_sources rs
    WHERE rs.org_id = o.id
) rag ON true
LEFT JOIN LATERAL (
    SELECT SUM(cle.amount) AS credit_balance
    FROM public.credit_ledger_entries cle
    WHERE cle.org_id = o.id
) credits ON true;

CREATE OR REPLACE VIEW public.observability_llm_hourly AS
SELECT
    date_trunc('hour', created_at) AS bucket,
    org_id,
    provider_id,
    COALESCE(model, 'unknown') AS model,
    COUNT(*) AS request_count,
    COUNT(*) FILTER (WHERE error_type IS NOT NULL OR upstream_status >= 400) AS error_count,
    COALESCE(SUM(input_tokens), 0) AS input_tokens,
    COALESCE(SUM(output_tokens), 0) AS output_tokens,
    COALESCE(SUM(cached_tokens), 0) AS cached_tokens,
    COALESCE(SUM(reasoning_tokens), 0) AS reasoning_tokens,
    COALESCE(SUM(cost), 0) AS cost,
    COALESCE(AVG(ttfb_ms), 0) AS avg_ttfb_ms,
    COALESCE(AVG(total_ms), 0) AS avg_total_ms
FROM public.generations
GROUP BY 1, 2, 3, 4;

CREATE OR REPLACE VIEW public.observability_tool_hourly AS
SELECT
    date_trunc('hour', created_at) AS bucket,
    org_id,
    tool_name,
    status,
    COUNT(*) AS call_count,
    COUNT(*) FILTER (WHERE error_message IS NOT NULL AND error_message <> '') AS error_count,
    COALESCE(AVG(total_ms), 0) AS avg_total_ms,
    COALESCE(SUM(credits_used), 0) AS credits_used
FROM public.tool_usages
GROUP BY 1, 2, 3, 4;

CREATE OR REPLACE VIEW public.observability_automation_runs AS
SELECT
    'schedule'::text AS automation_type,
    r.id AS run_id,
    r.org_id,
    r.agent_id,
    r.schedule_id AS automation_id,
    r.session_id,
    r.status,
    (r.error <> '') AS has_error,
    r.scheduled_at,
    r.started_at,
    r.completed_at,
    r.duration_ms,
    r.created_at
FROM public.agent_schedule_runs r
UNION ALL
SELECT
    'trigger'::text AS automation_type,
    d.id AS run_id,
    d.org_id,
    d.agent_id,
    d.trigger_id AS automation_id,
    d.session_id,
    'delivered'::text AS status,
    false AS has_error,
    NULL::timestamptz AS scheduled_at,
    d.created_at AS started_at,
    d.created_at AS completed_at,
    0::bigint AS duration_ms,
    d.created_at
FROM public.agent_trigger_deliveries d;

CREATE OR REPLACE VIEW public.observability_rag_health AS
SELECT
    rs.id AS source_id,
    rs.org_id,
    rs.kind,
    rs.status,
    rs.enabled,
    rs.in_repeated_error_state,
    rs.total_docs_indexed,
    rs.last_successful_index_time,
    rs.indexing_start,
    rs.created_at,
    rs.updated_at,
    COUNT(DISTINCT ria.id) FILTER (WHERE ria.status IN ('FAILED', 'failed')) AS failed_attempt_count,
    COUNT(DISTINCT rie.id) FILTER (WHERE NOT rie.is_resolved) AS unresolved_error_count,
    MAX(ria.time_updated) AS last_attempt_at
FROM public.rag_sources rs
LEFT JOIN public.rag_index_attempts ria ON ria.rag_source_id = rs.id
LEFT JOIN public.rag_index_attempt_errors rie ON rie.rag_source_id = rs.id
GROUP BY rs.id;

CREATE OR REPLACE VIEW public.observability_product_operations AS
SELECT
    'app'::text AS product,
    a.id AS resource_id,
    a.org_id,
    a.team_id,
    a.source_session_id AS session_id,
    a.status,
    (a.status = 'failed') AS has_error,
    a.created_at,
    a.updated_at
FROM public.apps a
WHERE a.archived_at IS NULL
UNION ALL
SELECT
    'sheet_import'::text AS product,
    j.id AS resource_id,
    j.org_id,
    NULL::uuid AS team_id,
    NULL::uuid AS session_id,
    j.status,
    (j.status = 'failed' OR j.error <> '') AS has_error,
    j.created_at,
    j.updated_at
FROM public.sheet_import_jobs j
UNION ALL
SELECT
    'email'::text AS product,
    m.id AS resource_id,
    m.org_id,
    NULL::uuid AS team_id,
    t.session_id,
    m.status,
    (m.status IN ('failed', 'bounced', 'complained')) AS has_error,
    m.created_at,
    m.updated_at
FROM public.agent_email_messages m
JOIN public.agent_email_threads t ON t.id = m.thread_id;

CREATE OR REPLACE VIEW public.observability_billing_health AS
SELECT
    o.id AS org_id,
    COALESCE(credits.credit_balance, 0) AS credit_balance,
    COALESCE(generations.unbilled_generation_count, 0) AS unbilled_generation_count,
    COALESCE(generations.unbilled_cost, 0) AS unbilled_cost,
    COALESCE(generations.billing_error_count, 0) AS billing_error_count,
    generations.oldest_unbilled_at,
    COALESCE(purchases.pending_purchase_count, 0) AS pending_purchase_count,
    COALESCE(purchases.failed_purchase_count, 0) AS failed_purchase_count
FROM public.orgs o
LEFT JOIN LATERAL (
    SELECT SUM(cle.amount) AS credit_balance
    FROM public.credit_ledger_entries cle
    WHERE cle.org_id = o.id
) credits ON true
LEFT JOIN LATERAL (
    SELECT
        COUNT(*) FILTER (WHERE g.billed_at IS NULL AND g.cost > 0) AS unbilled_generation_count,
        SUM(g.cost) FILTER (WHERE g.billed_at IS NULL) AS unbilled_cost,
        COUNT(*) FILTER (WHERE g.billing_error IS NOT NULL AND g.billing_error <> '') AS billing_error_count,
        MIN(g.created_at) FILTER (WHERE g.billed_at IS NULL AND g.cost > 0) AS oldest_unbilled_at
    FROM public.generations g
    WHERE g.org_id = o.id
) generations ON true
LEFT JOIN LATERAL (
    SELECT
        COUNT(*) FILTER (WHERE cp.status = 'pending') AS pending_purchase_count,
        COUNT(*) FILTER (WHERE cp.status = 'failed') AS failed_purchase_count
    FROM public.credit_purchases cp
    WHERE cp.org_id = o.id
) purchases ON true;

CREATE OR REPLACE VIEW public.observability_security_events AS
SELECT
    date_trunc('hour', created_at) AS bucket,
    org_id,
    action,
    COUNT(*) AS event_count,
    COUNT(DISTINCT credential_id) AS credential_count
FROM public.audit_log
GROUP BY 1, 2, 3;

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'hivy_observability') THEN
        GRANT USAGE ON SCHEMA public TO hivy_observability;
        GRANT SELECT ON
            public.observability_session_support,
            public.observability_org_support,
            public.observability_llm_hourly,
            public.observability_tool_hourly,
            public.observability_automation_runs,
            public.observability_rag_health,
            public.observability_product_operations,
            public.observability_billing_health,
            public.observability_security_events
        TO hivy_observability;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down

DROP VIEW IF EXISTS public.observability_security_events;
DROP VIEW IF EXISTS public.observability_billing_health;
DROP VIEW IF EXISTS public.observability_product_operations;
DROP VIEW IF EXISTS public.observability_rag_health;
DROP VIEW IF EXISTS public.observability_automation_runs;
DROP VIEW IF EXISTS public.observability_tool_hourly;
DROP VIEW IF EXISTS public.observability_llm_hourly;
DROP VIEW IF EXISTS public.observability_org_support;
DROP VIEW IF EXISTS public.observability_session_support;
