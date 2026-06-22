-- +goose Up
CREATE TABLE teams (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    created_by uuid,
    archived_at timestamp with time zone,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE team_members (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    team_id uuid NOT NULL,
    user_id uuid NOT NULL,
    role text DEFAULT 'member'::text NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE org_invite_teams (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    org_invite_id uuid NOT NULL,
    team_id uuid NOT NULL,
    created_at timestamp with time zone
);

CREATE TABLE agent_channels (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    channel_id uuid NOT NULL,
    created_at timestamp with time zone
);

ALTER TABLE teams ADD CONSTRAINT teams_pkey PRIMARY KEY (id);
ALTER TABLE team_members ADD CONSTRAINT team_members_pkey PRIMARY KEY (id);
ALTER TABLE org_invite_teams ADD CONSTRAINT org_invite_teams_pkey PRIMARY KEY (id);
ALTER TABLE agent_channels ADD CONSTRAINT agent_channels_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX idx_teams_org_name_active ON teams USING btree (org_id, name) WHERE archived_at IS NULL;
CREATE INDEX idx_teams_org_id ON teams USING btree (org_id);
CREATE INDEX idx_teams_archived_at ON teams USING btree (archived_at);

CREATE UNIQUE INDEX idx_team_members_team_user ON team_members USING btree (team_id, user_id);
CREATE INDEX idx_team_members_org_user ON team_members USING btree (org_id, user_id);
CREATE INDEX idx_team_members_team_id ON team_members USING btree (team_id);

CREATE UNIQUE INDEX idx_org_invite_teams_invite_team ON org_invite_teams USING btree (org_invite_id, team_id);
CREATE INDEX idx_org_invite_teams_org_id ON org_invite_teams USING btree (org_id);
CREATE INDEX idx_org_invite_teams_team_id ON org_invite_teams USING btree (team_id);

CREATE UNIQUE INDEX idx_agent_channels_agent_channel ON agent_channels USING btree (agent_id, channel_id);
CREATE INDEX idx_agent_channels_org_agent ON agent_channels USING btree (org_id, agent_id);
CREATE INDEX idx_agent_channels_channel_id ON agent_channels USING btree (channel_id);

ALTER TABLE channels ADD COLUMN IF NOT EXISTS team_id uuid;
CREATE INDEX idx_channels_team_id ON channels USING btree (team_id);

ALTER TABLE ONLY teams
    ADD CONSTRAINT fk_teams_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;
ALTER TABLE ONLY teams
    ADD CONSTRAINT fk_teams_creator FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE ONLY team_members
    ADD CONSTRAINT fk_team_members_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;
ALTER TABLE ONLY team_members
    ADD CONSTRAINT fk_team_members_team FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE;
ALTER TABLE ONLY team_members
    ADD CONSTRAINT fk_team_members_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY org_invite_teams
    ADD CONSTRAINT fk_org_invite_teams_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;
ALTER TABLE ONLY org_invite_teams
    ADD CONSTRAINT fk_org_invite_teams_invite FOREIGN KEY (org_invite_id) REFERENCES org_invites(id) ON DELETE CASCADE;
ALTER TABLE ONLY org_invite_teams
    ADD CONSTRAINT fk_org_invite_teams_team FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE;

ALTER TABLE ONLY agent_channels
    ADD CONSTRAINT fk_agent_channels_org FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;
ALTER TABLE ONLY agent_channels
    ADD CONSTRAINT fk_agent_channels_agent FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE;
ALTER TABLE ONLY agent_channels
    ADD CONSTRAINT fk_agent_channels_channel FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE;

ALTER TABLE ONLY channels
    ADD CONSTRAINT fk_channels_team FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE ONLY channels DROP CONSTRAINT IF EXISTS fk_channels_team;
DROP INDEX IF EXISTS idx_channels_team_id;
ALTER TABLE channels DROP COLUMN IF EXISTS team_id;

DROP TABLE IF EXISTS agent_channels;
DROP TABLE IF EXISTS org_invite_teams;
DROP TABLE IF EXISTS team_members;
DROP TABLE IF EXISTS teams;
