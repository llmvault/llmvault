-- +goose Up

-- This migration was historically deployed after the billing migration, then
-- accidentally folded into 000001. Keep every operation idempotent so both
-- databases that already have the current baseline and databases upgrading
-- from version 2 converge on the same schema.
-- +goose StatementBegin
DO $$
BEGIN
IF to_regclass('public.agents') IS NULL THEN
    RETURN;
END IF;

ALTER TABLE public.agents
    ADD COLUMN IF NOT EXISTS email_inbox_local_part text NOT NULL DEFAULT '';

UPDATE public.agents
SET email_inbox_local_part = 'agent-' || substr(replace(id::text, '-', ''), 1, 8)
WHERE email_inbox_local_part = '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_agents_email_inbox_local_part
    ON public.agents (email_inbox_local_part)
    WHERE email_inbox_local_part <> '';

CREATE TABLE IF NOT EXISTS public.agent_email_threads (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    session_id uuid,
    root_message_id text NOT NULL DEFAULT '',
    reply_token text NOT NULL UNIQUE,
    last_message_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT agent_email_threads_pkey PRIMARY KEY (id),
    CONSTRAINT fk_agent_email_threads_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_email_threads_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_email_threads_session FOREIGN KEY (session_id) REFERENCES public.sessions(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_agent_email_threads_org_agent ON public.agent_email_threads(org_id, agent_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_email_threads_reply_token ON public.agent_email_threads(reply_token);
CREATE INDEX IF NOT EXISTS idx_agent_email_threads_session ON public.agent_email_threads(session_id);

CREATE TABLE IF NOT EXISTS public.agent_email_messages (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    thread_id uuid NOT NULL,
    direction text NOT NULL,
    status text NOT NULL DEFAULT 'received',
    resend_email_id text NOT NULL DEFAULT '',
    message_id text NOT NULL DEFAULT '',
    in_reply_to text NOT NULL DEFAULT '',
    "references" jsonb NOT NULL DEFAULT '[]'::jsonb,
    from_address text NOT NULL DEFAULT '',
    to_addresses jsonb NOT NULL DEFAULT '[]'::jsonb,
    cc_addresses jsonb NOT NULL DEFAULT '[]'::jsonb,
    subject text NOT NULL DEFAULT '',
    text_body text NOT NULL DEFAULT '',
    html_body text NOT NULL DEFAULT '',
    headers jsonb NOT NULL DEFAULT '{}'::jsonb,
    provider_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT agent_email_messages_pkey PRIMARY KEY (id),
    CONSTRAINT fk_agent_email_messages_org FOREIGN KEY (org_id) REFERENCES public.orgs(id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_email_messages_agent FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE,
    CONSTRAINT fk_agent_email_messages_thread FOREIGN KEY (thread_id) REFERENCES public.agent_email_threads(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_email_messages_resend_id
    ON public.agent_email_messages(resend_email_id) WHERE resend_email_id <> '';
CREATE INDEX IF NOT EXISTS idx_agent_email_messages_agent_message_id
    ON public.agent_email_messages(agent_id, message_id) WHERE message_id <> '';
CREATE INDEX IF NOT EXISTS idx_agent_email_messages_thread_provider_at
    ON public.agent_email_messages(thread_id, provider_at);

CREATE TABLE IF NOT EXISTS public.agent_email_webhook_receipts (
    svix_id text NOT NULL,
    event_type text NOT NULL,
    resend_email_id text NOT NULL DEFAULT '',
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    processed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT agent_email_webhook_receipts_pkey PRIMARY KEY (svix_id)
);
CREATE INDEX IF NOT EXISTS idx_agent_email_webhook_receipts_resend_email_id
    ON public.agent_email_webhook_receipts(resend_email_id);

END $$;
-- +goose StatementEnd

-- +goose Down

DROP TABLE IF EXISTS public.agent_email_webhook_receipts;
DROP TABLE IF EXISTS public.agent_email_messages;
DROP TABLE IF EXISTS public.agent_email_threads;
DROP INDEX IF EXISTS public.idx_agents_email_inbox_local_part;
ALTER TABLE public.agents DROP COLUMN IF EXISTS email_inbox_local_part;
