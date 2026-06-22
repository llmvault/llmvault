ALTER TABLE agent_config
ADD COLUMN workspace_json TEXT NOT NULL DEFAULT '{"repos":[]}';
