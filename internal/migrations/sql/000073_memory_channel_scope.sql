-- +goose Up
-- Re-scope agent memories from (scope, agent_id, user_id) to a single
-- channel_id dimension: channel_id set = that channel's memory, NULL = org-wide.
-- Pre-launch, so no data migration — the old scoping columns and their
-- constraints/indexes are dropped outright.

ALTER TABLE agent_memories ADD COLUMN channel_id uuid;
ALTER TABLE agent_memories
    ADD CONSTRAINT fk_agent_memories_channel FOREIGN KEY (channel_id)
    REFERENCES channels(id) ON DELETE CASCADE;

-- Drop everything keyed on the retired scoping columns before the columns go.
DROP INDEX IF EXISTS idx_agent_memories_org_scope;
DROP INDEX IF EXISTS idx_agent_memories_org_user;
DROP INDEX IF EXISTS idx_agent_memories_org_agent;
DROP INDEX IF EXISTS idx_agent_memories_fingerprint;

ALTER TABLE agent_memories DROP CONSTRAINT IF EXISTS agent_memories_scope_check;
ALTER TABLE agent_memories DROP CONSTRAINT IF EXISTS agent_memories_scope_user_check;
ALTER TABLE agent_memories DROP CONSTRAINT IF EXISTS fk_agent_memories_agent;
ALTER TABLE agent_memories DROP CONSTRAINT IF EXISTS fk_agent_memories_user;

ALTER TABLE agent_memories DROP COLUMN scope;
ALTER TABLE agent_memories DROP COLUMN agent_id;
ALTER TABLE agent_memories DROP COLUMN user_id;

CREATE INDEX idx_agent_memories_org_channel ON agent_memories
    USING btree (org_id, channel_id, created_at DESC)
    WHERE archived_at IS NULL;

-- Fingerprint already embeds org+channel+content, so it is globally unique per
-- logical memory; the dedup index no longer needs a scope column.
CREATE UNIQUE INDEX idx_agent_memories_fingerprint ON agent_memories
    USING btree (memory_fingerprint)
    WHERE archived_at IS NULL AND memory_fingerprint <> '';

-- Per-channel switch: when true, org-wide (channel_id IS NULL) memories are
-- folded into that channel's recall/search for its agents.
ALTER TABLE channels ADD COLUMN expose_org_memories boolean NOT NULL DEFAULT true;

-- +goose Down
ALTER TABLE channels DROP COLUMN IF EXISTS expose_org_memories;

DROP INDEX IF EXISTS idx_agent_memories_fingerprint;
DROP INDEX IF EXISTS idx_agent_memories_org_channel;

ALTER TABLE agent_memories ADD COLUMN scope text NOT NULL DEFAULT 'org';
ALTER TABLE agent_memories ADD COLUMN agent_id uuid;
ALTER TABLE agent_memories ADD COLUMN user_id uuid;
ALTER TABLE agent_memories
    ADD CONSTRAINT agent_memories_scope_check CHECK (scope IN ('org', 'user'));
ALTER TABLE agent_memories
    ADD CONSTRAINT fk_agent_memories_agent FOREIGN KEY (agent_id)
    REFERENCES agents(id) ON DELETE SET NULL;
ALTER TABLE agent_memories
    ADD CONSTRAINT fk_agent_memories_user FOREIGN KEY (user_id)
    REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE agent_memories DROP CONSTRAINT IF EXISTS fk_agent_memories_channel;
ALTER TABLE agent_memories DROP COLUMN IF EXISTS channel_id;

CREATE UNIQUE INDEX idx_agent_memories_fingerprint ON agent_memories
    USING btree (scope, memory_fingerprint)
    WHERE archived_at IS NULL AND memory_fingerprint <> '';
