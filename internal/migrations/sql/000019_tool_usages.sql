-- +goose Up
CREATE TABLE public.tool_usages (
    id text NOT NULL,
    org_id uuid NOT NULL,
    agent_id text NOT NULL,
    token_jti text NOT NULL,
    tool_name text NOT NULL,
    input text,
    pages_returned bigint DEFAULT 0,
    status text NOT NULL,
    error_message text,
    total_ms bigint,
    credits_used bigint DEFAULT 0,
    ip_address inet,
    created_at timestamp with time zone NOT NULL
);

ALTER TABLE ONLY public.tool_usages
    ADD CONSTRAINT tool_usages_pkey PRIMARY KEY (id);

CREATE INDEX idx_tu_org_agent ON public.tool_usages USING btree (agent_id);

CREATE INDEX idx_tu_org_created ON public.tool_usages USING btree (org_id, created_at);
