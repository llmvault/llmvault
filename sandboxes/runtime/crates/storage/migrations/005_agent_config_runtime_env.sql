ALTER TABLE agent_config
ADD COLUMN runtime_env_json TEXT NOT NULL DEFAULT '{}';
