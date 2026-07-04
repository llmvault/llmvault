-- +goose Up
-- Maps a bot-created GitHub pull request back to the session that opened it, so
-- later events on that PR (comments/mentions, reviews, CI) route into the same
-- session. Populated by the gh-wrapper capture path (POST
-- /internal/github-pr-created/{agentID}).
CREATE TABLE github_pull_request_sessions (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     uuid NOT NULL,
    agent_id   uuid,
    repo       text NOT NULL,
    pr_number  integer NOT NULL,
    session_id uuid NOT NULL,
    head_ref   text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_github_pr_sessions_repo_number
    ON github_pull_request_sessions USING btree (org_id, repo, pr_number);
CREATE INDEX idx_github_pr_sessions_session
    ON github_pull_request_sessions USING btree (session_id);

-- +goose Down
DROP TABLE IF EXISTS github_pull_request_sessions;
