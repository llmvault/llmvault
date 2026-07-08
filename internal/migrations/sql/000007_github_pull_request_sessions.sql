-- +goose Up
CREATE TABLE public.github_pull_request_sessions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    agent_id uuid,
    repo text NOT NULL,
    pr_number integer NOT NULL,
    session_id uuid NOT NULL,
    head_ref text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.github_pull_request_sessions
    ADD CONSTRAINT github_pull_request_sessions_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX idx_github_pr_sessions_repo_number ON public.github_pull_request_sessions USING btree (org_id, repo, pr_number);

CREATE INDEX idx_github_pr_sessions_session ON public.github_pull_request_sessions USING btree (session_id);

-- +goose Down
DROP TABLE IF EXISTS public.github_pull_request_sessions CASCADE;
