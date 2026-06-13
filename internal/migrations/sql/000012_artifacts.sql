-- +goose Up
-- Session artifacts

CREATE TABLE artifacts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    session_id uuid,
    agent_id uuid,
    created_by uuid,
    kind text NOT NULL,
    title text DEFAULT ''::text NOT NULL,
    storage text NOT NULL,
    s3_key text,
    content jsonb,
    preview_url text,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE ONLY artifacts ADD CONSTRAINT artifacts_pkey PRIMARY KEY (id);
CREATE INDEX idx_artifacts_org_id ON artifacts USING btree (org_id);
CREATE INDEX idx_artifacts_session ON artifacts USING btree (session_id, created_at DESC);
CREATE INDEX idx_artifacts_agent_id ON artifacts USING btree (agent_id);
CREATE INDEX idx_artifacts_created_by ON artifacts USING btree (created_by);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'baseline down migration is intentionally unsupported; reset or restore the database instead'; END $$;
-- +goose StatementEnd
