-- +goose Up
INSERT INTO rag_sources (
  id,
  org_id,
  kind,
  name,
  status,
  enabled,
  config,
  access_type,
  refresh_freq_seconds,
  total_docs_indexed,
  in_repeated_error_state,
  created_at,
  updated_at
)
SELECT
  gen_random_uuid(),
  o.id,
  'WEBSITE',
  btrim(o.website),
  'INITIAL_INDEXING',
  true,
  jsonb_build_object('url', btrim(o.website), 'max_pages', 100),
  'public',
  86400,
  0,
  false,
  now(),
  now()
FROM orgs o
WHERE o.active = true
  AND o.onboarded = true
  AND btrim(o.website) <> ''
  AND NOT EXISTS (
    SELECT 1
    FROM rag_sources s
    WHERE s.org_id = o.id
      AND s.kind = 'WEBSITE'
      AND s.config->>'url' = btrim(o.website)
  );

-- +goose Down
-- No-op: backfilled RAG sources may be indexed or edited after creation.
SELECT 1;
