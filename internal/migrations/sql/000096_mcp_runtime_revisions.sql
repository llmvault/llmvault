-- +goose Up
-- Every runtime keeps a short-lived copy of resolved MCP credentials. A
-- monotonic org revision lets the next turn detect any definition,
-- authorization, or assignment change and reload before tool use.
ALTER TABLE public.orgs
    ADD COLUMN mcp_config_version bigint NOT NULL DEFAULT 0;

ALTER TABLE public.sessions
    ADD COLUMN runtime_mcp_config_version bigint NOT NULL DEFAULT 0;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION public.bump_mcp_config_version()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    target_org_id uuid;
BEGIN
    target_org_id := CASE WHEN TG_OP = 'DELETE' THEN OLD.org_id ELSE NEW.org_id END;
    UPDATE public.orgs
       SET mcp_config_version = mcp_config_version + 1
     WHERE id = target_org_id;
    RETURN CASE WHEN TG_OP = 'DELETE' THEN OLD ELSE NEW END;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER mcp_servers_config_version
AFTER INSERT OR UPDATE OR DELETE ON public.mcp_servers
FOR EACH ROW EXECUTE FUNCTION public.bump_mcp_config_version();

CREATE TRIGGER mcp_authorizations_config_version
AFTER INSERT OR UPDATE OR DELETE ON public.mcp_authorizations
FOR EACH ROW EXECUTE FUNCTION public.bump_mcp_config_version();

CREATE TRIGGER team_mcp_servers_config_version
AFTER INSERT OR UPDATE OR DELETE ON public.team_mcp_servers
FOR EACH ROW EXECUTE FUNCTION public.bump_mcp_config_version();

CREATE TRIGGER agent_mcp_servers_config_version
AFTER INSERT OR UPDATE OR DELETE ON public.agent_mcp_servers
FOR EACH ROW EXECUTE FUNCTION public.bump_mcp_config_version();

CREATE TRIGGER user_agent_mcp_servers_config_version
AFTER INSERT OR UPDATE OR DELETE ON public.user_agent_mcp_servers
FOR EACH ROW EXECUTE FUNCTION public.bump_mcp_config_version();
