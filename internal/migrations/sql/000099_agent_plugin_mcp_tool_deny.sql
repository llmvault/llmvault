-- +goose Up
ALTER TABLE public.agents
    ADD COLUMN plugin_mcp_tool_deny jsonb DEFAULT '{}'::jsonb NOT NULL;

-- +goose Down
ALTER TABLE public.agents
    DROP COLUMN plugin_mcp_tool_deny;
