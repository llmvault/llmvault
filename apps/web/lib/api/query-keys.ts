/**
 * Shared React Query key builders for the generated `$api` layer.
 *
 * openapi-react-query derives its query keys as `[method, path, options?]`, and
 * TanStack matches from the start of the array, so a `[method, path]` prefix key
 * invalidates every query for that endpoint regardless of params. These builders
 * are the single source of truth for those literals — never hand-write
 * `["get", "/v1/..."]` inline (a path rename would then silently miss callers).
 *
 * Full keys that must line up with a specific `$api.useQuery(...)` call
 * (i.e. include params) live next to that query — these are the invalidation
 * prefixes shared across the app.
 */
export const queryKeys = {
  authMe: () => ["get", "/auth/me"] as const,
  orgCurrent: () => ["get", "/v1/orgs/current"] as const,
  team: () => ["get", "/v1/orgs/current/teams/{id}"] as const,
  teams: () => ["get", "/v1/orgs/current/teams"] as const,
  teamEnvironmentVariables: () =>
    ["get", "/v1/orgs/current/teams/{id}/environment-variables"] as const,
  channels: () => ["get", "/v1/channels"] as const,
  channel: () => ["get", "/v1/channels/{id}"] as const,
  channelSessions: () => ["get", "/v1/channels/{id}/sessions"] as const,
  sessions: () => ["get", "/v1/sessions"] as const,
  sessionUsage: (id: string) =>
    ["get", "/v1/sessions/{id}/usage", { params: { path: { id } } }] as const,
  connections: () => ["get", "/v1/connections"] as const,
  skills: () => ["get", "/v1/skills"] as const,
  databaseIntegrations: () => ["get", "/v1/database-integrations"] as const,
  mcpServers: () => ["get", "/v1/mcp-servers"] as const,
  mcpServer: (id: string) =>
    ["get", "/v1/mcp-servers/{id}", { params: { path: { id } } }] as const,
  teamMcpServers: (teamID: string) =>
    [
      "get",
      "/v1/orgs/current/teams/{teamID}/mcp-servers",
      { params: { path: { teamID } } },
    ] as const,
  agentMcpServers: (agentID: string) =>
    [
      "get",
      "/v1/agents/{agentID}/mcp-servers",
      { params: { path: { agentID } } },
    ] as const,
  agentPersonalMcpServers: (agentID: string) =>
    [
      "get",
      "/v1/agents/{agentID}/personal-mcp-servers",
      { params: { path: { agentID } } },
    ] as const,
  triggers: () => ["get", "/v1/triggers"] as const,
  trigger: () => ["get", "/v1/triggers/{id}"] as const,
  agents: () => ["get", "/v1/agents"] as const,
  agent: () => ["get", "/v1/agents/{id}"] as const,
  agentCatalog: () => ["get", "/v1/agents/catalog/{slug}"] as const,
  schedules: () => ["get", "/v1/schedules"] as const,
  schedule: () => ["get", "/v1/schedules/{id}"] as const,
  billingSubscription: () => ["get", "/v1/billing/subscription"] as const,
  usage: () => ["get", "/v1/usage"] as const,
  dashboard: () => ["get", "/v1/dashboard"] as const,
  canvasProjects: () => ["get", "/v1/canvas/projects"] as const,
  canvasArtifacts: () => ["get", "/v1/canvas/artifacts"] as const,
  canvasArtifact: () => ["get", "/v1/canvas/artifacts/{artifactID}"] as const,
} as const
