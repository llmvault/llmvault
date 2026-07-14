-- +goose Up
CREATE TABLE public.mcp_servers (
    id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
    org_id uuid NOT NULL REFERENCES public.orgs(id) ON DELETE CASCADE,
    scope text NOT NULL,
    owner_user_id uuid REFERENCES public.users(id) ON DELETE CASCADE,
    slug text NOT NULL,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    url text NOT NULL,
    transport text NOT NULL DEFAULT 'streamable_http',
    auth_type text NOT NULL DEFAULT 'none',
    authorization_policy text NOT NULL DEFAULT 'none',
    header_name text NOT NULL DEFAULT '',
    oauth_metadata jsonb NOT NULL DEFAULT '{}',
    status text NOT NULL DEFAULT 'active',
    created_by_user_id uuid REFERENCES public.users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT mcp_servers_scope_check CHECK (
        (scope = 'personal' AND owner_user_id IS NOT NULL)
        OR (scope = 'org' AND owner_user_id IS NULL)
    ),
    CONSTRAINT mcp_servers_transport_check CHECK (transport IN ('streamable_http', 'sse')),
    CONSTRAINT mcp_servers_auth_type_check CHECK (auth_type IN ('none', 'static_bearer', 'static_header', 'oauth_authorization_code', 'oauth_client_credentials')),
    CONSTRAINT mcp_servers_authorization_policy_check CHECK (authorization_policy IN ('none', 'user_required', 'service_required', 'prefer_user', 'prefer_service')),
    CONSTRAINT mcp_servers_status_check CHECK (status IN ('active', 'disabled'))
);

CREATE INDEX idx_mcp_servers_org ON public.mcp_servers(org_id);
CREATE INDEX idx_mcp_servers_owner ON public.mcp_servers(owner_user_id);
CREATE UNIQUE INDEX idx_mcp_servers_org_slug ON public.mcp_servers(org_id, slug) WHERE scope = 'org';
CREATE UNIQUE INDEX idx_mcp_servers_personal_slug ON public.mcp_servers(org_id, owner_user_id, slug) WHERE scope = 'personal';
CREATE UNIQUE INDEX idx_mcp_servers_id_org ON public.mcp_servers(id, org_id);

CREATE TABLE public.mcp_authorizations (
    id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
    org_id uuid NOT NULL REFERENCES public.orgs(id) ON DELETE CASCADE,
    mcp_server_id uuid NOT NULL REFERENCES public.mcp_servers(id) ON DELETE CASCADE,
    principal_type text NOT NULL,
    principal_user_id uuid REFERENCES public.users(id) ON DELETE CASCADE,
    auth_type text NOT NULL,
    credentials_encrypted bytea NOT NULL,
    client_id text NOT NULL DEFAULT '',
    scopes text[] NOT NULL DEFAULT '{}',
    token_type text NOT NULL DEFAULT '',
    expires_at timestamptz,
    refresh_expires_at timestamptz,
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT mcp_authorizations_principal_check CHECK (
        (principal_type = 'user' AND principal_user_id IS NOT NULL)
        OR (principal_type = 'org_service' AND principal_user_id IS NULL)
    ),
    CONSTRAINT mcp_authorizations_auth_type_check CHECK (auth_type IN ('none', 'static_bearer', 'static_header', 'oauth_authorization_code', 'oauth_client_credentials')),
    CONSTRAINT mcp_authorizations_status_check CHECK (status IN ('active', 'expired', 'revoked')),
    CONSTRAINT mcp_authorizations_server_org_fkey FOREIGN KEY (mcp_server_id, org_id) REFERENCES public.mcp_servers(id, org_id) ON DELETE CASCADE
);

CREATE INDEX idx_mcp_authorizations_org ON public.mcp_authorizations(org_id);
CREATE INDEX idx_mcp_authorizations_server ON public.mcp_authorizations(mcp_server_id);
CREATE UNIQUE INDEX idx_mcp_authorizations_user ON public.mcp_authorizations(mcp_server_id, principal_user_id) WHERE principal_type = 'user';
CREATE UNIQUE INDEX idx_mcp_authorizations_service ON public.mcp_authorizations(mcp_server_id) WHERE principal_type = 'org_service';

CREATE TABLE public.mcp_oauth_states (
    id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
    org_id uuid NOT NULL REFERENCES public.orgs(id) ON DELETE CASCADE,
    mcp_server_id uuid NOT NULL REFERENCES public.mcp_servers(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    principal_type text NOT NULL CHECK (principal_type IN ('user', 'org_service')),
    state_hash bytea NOT NULL UNIQUE,
    encrypted_verifier bytea NOT NULL,
    redirect_after text NOT NULL DEFAULT '',
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT mcp_oauth_states_server_org_fkey FOREIGN KEY (mcp_server_id, org_id) REFERENCES public.mcp_servers(id, org_id) ON DELETE CASCADE
);
CREATE INDEX idx_mcp_oauth_states_org ON public.mcp_oauth_states(org_id);
CREATE INDEX idx_mcp_oauth_states_server ON public.mcp_oauth_states(mcp_server_id);
CREATE INDEX idx_mcp_oauth_states_expires ON public.mcp_oauth_states(expires_at);

CREATE UNIQUE INDEX idx_agents_id_org_id ON public.agents(id, org_id);

CREATE TABLE public.team_mcp_servers (
    id uuid DEFAULT gen_random_uuid() PRIMARY KEY,
    org_id uuid NOT NULL REFERENCES public.orgs(id) ON DELETE CASCADE,
    team_id uuid NOT NULL,
    mcp_server_id uuid NOT NULL,
    granted_by uuid REFERENCES public.users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT team_mcp_servers_team_org_fkey FOREIGN KEY (team_id, org_id) REFERENCES public.teams(id, org_id) ON DELETE CASCADE,
    CONSTRAINT team_mcp_servers_server_org_fkey FOREIGN KEY (mcp_server_id, org_id) REFERENCES public.mcp_servers(id, org_id) ON DELETE CASCADE,
    CONSTRAINT team_mcp_servers_unique UNIQUE (team_id, mcp_server_id)
);
CREATE INDEX idx_team_mcp_servers_org_team ON public.team_mcp_servers(org_id, team_id);
CREATE INDEX idx_team_mcp_servers_server ON public.team_mcp_servers(mcp_server_id);

CREATE TABLE public.agent_mcp_servers (
    org_id uuid NOT NULL REFERENCES public.orgs(id) ON DELETE CASCADE,
    agent_id uuid NOT NULL REFERENCES public.agents(id) ON DELETE CASCADE,
    mcp_server_id uuid NOT NULL REFERENCES public.mcp_servers(id) ON DELETE CASCADE,
    state text NOT NULL CHECK (state IN ('enabled', 'disabled')),
    updated_by uuid REFERENCES public.users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (agent_id, mcp_server_id),
    CONSTRAINT agent_mcp_servers_agent_org_fkey FOREIGN KEY (agent_id, org_id) REFERENCES public.agents(id, org_id) ON DELETE CASCADE,
    CONSTRAINT agent_mcp_servers_server_org_fkey FOREIGN KEY (mcp_server_id, org_id) REFERENCES public.mcp_servers(id, org_id) ON DELETE CASCADE
);
CREATE INDEX idx_agent_mcp_servers_org ON public.agent_mcp_servers(org_id);
CREATE INDEX idx_agent_mcp_servers_server ON public.agent_mcp_servers(mcp_server_id);

CREATE TABLE public.user_agent_mcp_servers (
    org_id uuid NOT NULL REFERENCES public.orgs(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    agent_id uuid NOT NULL REFERENCES public.agents(id) ON DELETE CASCADE,
    mcp_server_id uuid NOT NULL REFERENCES public.mcp_servers(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, agent_id, mcp_server_id),
    CONSTRAINT user_agent_mcp_servers_agent_org_fkey FOREIGN KEY (agent_id, org_id) REFERENCES public.agents(id, org_id) ON DELETE CASCADE,
    CONSTRAINT user_agent_mcp_servers_server_org_fkey FOREIGN KEY (mcp_server_id, org_id) REFERENCES public.mcp_servers(id, org_id) ON DELETE CASCADE
);
CREATE INDEX idx_user_agent_mcp_servers_org ON public.user_agent_mcp_servers(org_id);
CREATE INDEX idx_user_agent_mcp_servers_agent ON public.user_agent_mcp_servers(agent_id);
CREATE INDEX idx_user_agent_mcp_servers_server ON public.user_agent_mcp_servers(mcp_server_id);

ALTER TABLE public.agent_schedules
    ADD COLUMN created_by_user_id uuid REFERENCES public.users(id) ON DELETE SET NULL;
CREATE INDEX idx_agent_schedules_created_by_user_id ON public.agent_schedules(created_by_user_id);

ALTER TABLE public.sessions
    ADD COLUMN runtime_mcp_actor_user_id uuid REFERENCES public.users(id) ON DELETE SET NULL;
CREATE INDEX idx_sessions_runtime_mcp_actor_user_id ON public.sessions(runtime_mcp_actor_user_id);
