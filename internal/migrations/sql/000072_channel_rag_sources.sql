-- +goose Up
-- Channel-scoped knowledge access. Grants a channel access to a subset of the
-- org's RAG sources. When an agent calls search_knowledge_base from a session,
-- the search is restricted to the sources granted to that session's channel.
-- Deny-by-default: a channel with no rows here gets zero knowledge results.
CREATE TABLE channel_rag_sources (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        uuid NOT NULL,
    channel_id    uuid NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    rag_source_id uuid NOT NULL REFERENCES rag_sources(id) ON DELETE CASCADE,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_channel_rag_sources_channel_source ON channel_rag_sources (channel_id, rag_source_id);
CREATE INDEX idx_channel_rag_sources_channel ON channel_rag_sources (channel_id);
CREATE INDEX idx_channel_rag_sources_source ON channel_rag_sources (rag_source_id);
CREATE INDEX idx_channel_rag_sources_org ON channel_rag_sources (org_id);

-- +goose Down
DROP TABLE IF EXISTS channel_rag_sources;
