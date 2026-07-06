-- +goose Up
-- Observations are the consolidated memory product built from raw reflection
-- facts (agent_memories): one canonical observation per facet, with proof
-- counts, supersession, expiry, and channel scoping (channel_id set = that
-- channel's memory, NULL = org-wide). Facts stay as internal evidence;
-- observations are what agents and users see.

CREATE TABLE agent_observations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL,
    channel_id uuid,
    content text NOT NULL,
    kind text NOT NULL,
    entities text[] NOT NULL DEFAULT '{}',
    proof_count int NOT NULL DEFAULT 1,
    source_fact_ids uuid[] NOT NULL DEFAULT '{}',
    occurred_start timestamp with time zone,
    occurred_end timestamp with time zone,
    last_mentioned_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone,
    superseded_by uuid,
    archived_at timestamp with time zone,
    human_verified boolean NOT NULL DEFAULT false,
    embedding vector(1024),
    embedding_model text NOT NULL DEFAULT '',
    embedding_status text NOT NULL DEFAULT 'pending',
    embedding_revision integer NOT NULL DEFAULT 1,
    embedding_error text NOT NULL DEFAULT '',
    embedded_at timestamp with time zone,
    metadata jsonb NOT NULL DEFAULT '{}',
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now(),
    CONSTRAINT agent_observations_embedding_status_check
        CHECK (embedding_status IN ('pending', 'ready', 'failed'))
);

ALTER TABLE ONLY agent_observations
    ADD CONSTRAINT fk_agent_observations_org FOREIGN KEY (org_id)
    REFERENCES orgs(id) ON DELETE CASCADE;
ALTER TABLE ONLY agent_observations
    ADD CONSTRAINT fk_agent_observations_channel FOREIGN KEY (channel_id)
    REFERENCES channels(id) ON DELETE CASCADE;
ALTER TABLE ONLY agent_observations
    ADD CONSTRAINT fk_agent_observations_superseded_by FOREIGN KEY (superseded_by)
    REFERENCES agent_observations(id) ON DELETE SET NULL;

CREATE INDEX idx_agent_observations_org_channel ON agent_observations
    USING btree (org_id, channel_id)
    WHERE archived_at IS NULL;
CREATE INDEX idx_agent_observations_expires ON agent_observations
    USING btree (expires_at)
    WHERE archived_at IS NULL AND expires_at IS NOT NULL;
CREATE INDEX idx_agent_observations_embedding_hnsw ON agent_observations
    USING hnsw (embedding vector_cosine_ops)
    WHERE archived_at IS NULL AND embedding_status = 'ready';

-- Precomputed, byte-budgeted memory block per channel: written by the
-- consolidation worker (write-time ranking), read as one point-read on
-- session create. Org-wide observations are folded INTO each channel digest
-- at write time — there is no separate org digest row.
CREATE TABLE channel_memory_digests (
    channel_id uuid PRIMARY KEY,
    org_id uuid NOT NULL,
    content text NOT NULL,
    observation_count int NOT NULL DEFAULT 0,
    updated_at timestamp with time zone NOT NULL DEFAULT now()
);

ALTER TABLE ONLY channel_memory_digests
    ADD CONSTRAINT fk_channel_memory_digests_channel FOREIGN KEY (channel_id)
    REFERENCES channels(id) ON DELETE CASCADE;
ALTER TABLE ONLY channel_memory_digests
    ADD CONSTRAINT fk_channel_memory_digests_org FOREIGN KEY (org_id)
    REFERENCES orgs(id) ON DELETE CASCADE;

-- Content fingerprints of user-deleted memories: checked at
-- consolidation-create time so a deleted observation cannot be resurrected
-- by the next batch of facts.
CREATE TABLE memory_suppressions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL,
    channel_id uuid,
    content_fingerprint text NOT NULL,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    CONSTRAINT memory_suppressions_unique UNIQUE (org_id, channel_id, content_fingerprint)
);

ALTER TABLE ONLY memory_suppressions
    ADD CONSTRAINT fk_memory_suppressions_org FOREIGN KEY (org_id)
    REFERENCES orgs(id) ON DELETE CASCADE;
ALTER TABLE ONLY memory_suppressions
    ADD CONSTRAINT fk_memory_suppressions_channel FOREIGN KEY (channel_id)
    REFERENCES channels(id) ON DELETE CASCADE;

-- Facts (agent_memories) now carry a consolidation cursor: NULL = not yet
-- folded into observations. The consolidation worker and the stranded-facts
-- sweep both key off this column.
ALTER TABLE agent_memories ADD COLUMN consolidated_at timestamp with time zone;

CREATE INDEX idx_agent_memories_unconsolidated ON agent_memories
    USING btree (org_id, channel_id, created_at ASC)
    WHERE archived_at IS NULL AND consolidated_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_agent_memories_unconsolidated;
ALTER TABLE agent_memories DROP COLUMN IF EXISTS consolidated_at;

DROP TABLE IF EXISTS memory_suppressions;
DROP TABLE IF EXISTS channel_memory_digests;
DROP TABLE IF EXISTS agent_observations;
