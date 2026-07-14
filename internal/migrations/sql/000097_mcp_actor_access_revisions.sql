-- +goose Up
-- Actor and team membership changes can revoke a user's right to load personal
-- credentials even when no MCP definition or assignment changed.
CREATE TRIGGER org_memberships_mcp_config_version
AFTER INSERT OR UPDATE OR DELETE ON public.org_memberships
FOR EACH ROW EXECUTE FUNCTION public.bump_mcp_config_version();

CREATE TRIGGER team_members_mcp_config_version
AFTER INSERT OR UPDATE OR DELETE ON public.team_members
FOR EACH ROW EXECUTE FUNCTION public.bump_mcp_config_version();

CREATE TRIGGER agents_mcp_config_version
AFTER UPDATE OF team_id, status ON public.agents
FOR EACH ROW EXECUTE FUNCTION public.bump_mcp_config_version();

CREATE TRIGGER teams_mcp_config_version
AFTER UPDATE OF archived_at ON public.teams
FOR EACH ROW EXECUTE FUNCTION public.bump_mcp_config_version();
