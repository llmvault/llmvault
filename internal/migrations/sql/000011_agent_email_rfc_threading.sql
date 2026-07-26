-- +goose Up

-- New outbound email threads are correlated through the provider-assigned RFC
-- Message-ID and their originating Hivy session. Keep the unused legacy column
-- populated during this rollout so old and new application pods can overlap
-- without an old pod failing to scan a thread created by new code.
ALTER TABLE public.agent_email_threads
    ALTER COLUMN reply_token SET DEFAULT replace(gen_random_uuid()::text, '-', '');

-- +goose Down

ALTER TABLE public.agent_email_threads
    ALTER COLUMN reply_token DROP DEFAULT;
