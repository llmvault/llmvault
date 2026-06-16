ALTER TABLE sessions
    ADD COLUMN IF NOT EXISTS session_name_auto_generated_at timestamptz;
