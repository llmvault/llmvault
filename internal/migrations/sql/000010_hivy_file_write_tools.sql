-- +goose Up

-- File creation and patching are baseline capabilities for every team's
-- default Hivy agent. Preserve all existing runtime-tool choices while
-- repairing agents created from older catalog snapshots.
UPDATE public.agents
SET
    tools = tools || '{"write_file": true, "apply_patch": true}'::jsonb,
    updated_at = NOW()
WHERE is_default = TRUE
  AND parent_agent_id IS NULL
  AND status <> 'archived';

-- +goose Down

-- This capability repair is intentionally irreversible. Removing these keys on
-- rollback could revoke tools that were explicitly granted before this change.
SELECT 1;
