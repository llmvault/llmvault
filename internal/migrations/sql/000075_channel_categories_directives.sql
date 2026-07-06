-- +goose Up
-- Channel categories + memory missions (memory rework phase 3). Category is
-- the required create-time signal that seeds a curated per-channel memory
-- mission; existing channels intentionally stay 'general' (no backfill —
-- they opt into a category from channel settings whenever they want).
ALTER TABLE channels ADD COLUMN category text NOT NULL DEFAULT 'general';
ALTER TABLE channels ADD COLUMN memory_mission text;

-- Directives: hard rules injected verbatim into every agent prompt in scope
-- (channel_id set = that channel, NULL = org-wide). They exist only through
-- explicit human action: written by hand ('user-pinned') or promoted from an
-- extracted observation on user confirm ('extracted-confirmed').
CREATE TABLE agent_directives (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    channel_id uuid REFERENCES channels(id) ON DELETE CASCADE,
    content text NOT NULL,
    created_by_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    source text NOT NULL DEFAULT 'user-pinned',
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT agent_directives_source_check
        CHECK (source IN ('user-pinned', 'extracted-confirmed'))
);

CREATE INDEX idx_agent_directives_org_channel ON agent_directives
    USING btree (org_id, channel_id)
    WHERE active;

-- +goose Down
DROP TABLE IF EXISTS agent_directives;

ALTER TABLE channels DROP COLUMN IF EXISTS memory_mission;
ALTER TABLE channels DROP COLUMN IF EXISTS category;
