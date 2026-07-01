-- +goose Up
-- User-created agents (and their sub-agents) can carry their own MCP tool
-- filter (allow/deny) to control access to MCP tools like generate_image,
-- generate_vector_image, web_search, and web_fetch. NULL = no filter = all
-- MCP tools allowed.
ALTER TABLE agents
    ADD COLUMN mcp_tool_filter jsonb;

-- +goose Down
ALTER TABLE agents
    DROP COLUMN IF EXISTS mcp_tool_filter;
