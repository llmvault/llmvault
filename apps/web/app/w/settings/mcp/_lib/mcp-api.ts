import { api } from "@/lib/api/client"
import { extractErrorMessage } from "@/lib/api/error"

export type McpServerScope = "personal" | "org"
export type McpTransport = "streamable_http" | "sse"
export type McpAuthType =
  | "none"
  | "static_bearer"
  | "static_header"
  | "oauth_authorization_code"
  | "oauth_client_credentials"

export type McpAuthorizationPolicy =
  | "none"
  | "user_required"
  | "service_required"
  | "prefer_user"
  | "prefer_service"

export type McpAuthStatus =
  | "not_required"
  | "not_connected"
  | "pending"
  | "connected"
  | "expired"
  | "error"

export type McpHealthStatus =
  | "unknown"
  | "checking"
  | "healthy"
  | "degraded"
  | "unhealthy"

export interface McpServer {
  id: string
  name: string
  slug: string
  url: string
  scope: McpServerScope
  transport: McpTransport
  authType: McpAuthType
  authorizationPolicy: McpAuthorizationPolicy
  authStatus: McpAuthStatus
  healthStatus: McpHealthStatus
  secretSet: boolean
  teamIds: string[]
  agentIds: string[]
  toolCount: number | null
  authorizationLabel: string
  headerName: string
  tokenEndpoint: string
  lastCheckedAt: string | null
}

export interface CreateMcpServerInput {
  scope: McpServerScope
  name: string
  slug?: string
  url: string
  transport: McpTransport
  auth_type: McpAuthType
  authorization_policy: McpAuthorizationPolicy
  header_name?: string
  oauth_metadata?: {
    token_endpoint?: string
  }
  authorization?: McpAuthorizationInput
}

export interface McpAuthorizationInput {
  principal_type: "user" | "org_service"
  bearer_token?: string
  header_name?: string
  header_value?: string
  client_id?: string
  client_secret?: string
  scopes?: string[]
}

interface McpAuthorizationResult {
  authorizationUrl: string | null
  authStatus: McpAuthStatus
}

interface McpConnectionTest {
  connected: boolean
  protocolVersion: string
  serverName: string
  serverVersion: string
  capabilities: Record<string, unknown>
}

type ApiResult = {
  data?: unknown
  error?: unknown
}

function record(value: unknown): Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {}
}

function stringValue(value: unknown): string {
  return typeof value === "string" ? value : ""
}

function stringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  return value.filter((item): item is string => typeof item === "string")
}

function authType(value: unknown): McpAuthType {
  return value === "static_bearer" ||
    value === "static_header" ||
    value === "oauth_authorization_code" ||
    value === "oauth_client_credentials"
    ? value
    : "none"
}

function authorizationPolicy(value: unknown): McpAuthorizationPolicy {
  return value === "user_required" ||
    value === "service_required" ||
    value === "prefer_user" ||
    value === "prefer_service"
    ? value
    : "none"
}

function authStatus(value: unknown, type: McpAuthType): McpAuthStatus {
  if (
    value === "not_required" ||
    value === "not_connected" ||
    value === "pending" ||
    value === "connected" ||
    value === "expired" ||
    value === "error"
  ) {
    return value
  }
  if (value === "active") return "connected"
  if (value === "revoked") return "not_connected"
  return type === "none" ? "not_required" : "not_connected"
}

function healthStatus(value: unknown): McpHealthStatus {
  if (
    value === "checking" ||
    value === "healthy" ||
    value === "degraded" ||
    value === "unhealthy"
  ) {
    return value
  }
  return "unknown"
}

/**
 * Deliberately projects only display-safe fields. Even if a buggy server
 * includes plaintext token/header fields, they never enter the UI model.
 */
export function normalizeMcpServer(value: unknown): McpServer {
  const raw = record(value)
  const type = authType(raw.auth_type)
  const userAuthorization = record(raw.user_authorization)
  const serviceAuthorization = record(raw.service_authorization)
  const shownAuthorization =
    Object.keys(userAuthorization).length > 0
      ? userAuthorization
      : serviceAuthorization
  const shownAuthStatus = raw.auth_status ?? shownAuthorization.status
  return {
    id: stringValue(raw.id),
    name: stringValue(raw.name) || "Untitled MCP server",
    slug: stringValue(raw.slug),
    url: stringValue(raw.url) || stringValue(raw.endpoint_url),
    scope: raw.scope === "org" ? "org" : "personal",
    transport: raw.transport === "sse" ? "sse" : "streamable_http",
    authType: type,
    authorizationPolicy: authorizationPolicy(raw.authorization_policy),
    authStatus: authStatus(shownAuthStatus, type),
    healthStatus: healthStatus(raw.health_status ?? raw.status),
    secretSet:
      raw.secret_set === true || shownAuthorization.configured === true,
    teamIds: stringArray(raw.team_ids),
    agentIds: stringArray(raw.agent_ids),
    toolCount:
      typeof raw.tool_count === "number" && Number.isFinite(raw.tool_count)
        ? raw.tool_count
        : null,
    authorizationLabel:
      stringValue(raw.authorization_label) ||
      stringValue(raw.principal_label) ||
      (shownAuthorization.principal_type === "org_service"
        ? "Organization service identity"
        : shownAuthorization.configured === true
          ? "Your account"
          : ""),
    headerName: stringValue(raw.header_name),
    tokenEndpoint: stringValue(record(raw.oauth_metadata).token_endpoint),
    lastCheckedAt: stringValue(raw.last_checked_at) || null,
  }
}

function normalizeMcpServerList(value: unknown): McpServer[] {
  const raw = record(value)
  const items = Array.isArray(value)
    ? value
    : Array.isArray(raw.mcp_servers)
      ? raw.mcp_servers
      : Array.isArray(raw.data)
        ? raw.data
        : []
  return items.map(normalizeMcpServer).filter((server) => server.id !== "")
}

async function unwrap<T extends ApiResult>(
  result: Promise<T>,
  fallback: string
): Promise<unknown> {
  const { data, error } = await result
  if (error !== undefined) {
    throw new Error(extractErrorMessage(error, fallback))
  }
  return data
}

export async function listMcpServers(
  signal?: AbortSignal
): Promise<McpServer[]> {
  const servers: McpServer[] = []
  const seenCursors = new Set<string>()
  let cursor = ""
  do {
    const data = await unwrap(
      api.GET("/v1/mcp-servers", {
        params: { query: { limit: 100, cursor: cursor || undefined } },
        signal,
      }),
      "Could not load MCP servers"
    )
    servers.push(...normalizeMcpServerList(data))
    const nextCursor = stringValue(record(data).next_cursor)
    if (!nextCursor || seenCursors.has(nextCursor)) break
    seenCursors.add(nextCursor)
    cursor = nextCursor
  } while (cursor)
  return servers
}

export async function createMcpServer(
  input: CreateMcpServerInput
): Promise<McpServer> {
  const data = await unwrap(
    api.POST("/v1/mcp-servers", { body: input }),
    "Could not add MCP server"
  )
  return normalizeMcpServer(
    record(data).mcp_server ?? record(data).server ?? data
  )
}

export async function deleteMcpServer(id: string): Promise<void> {
  await unwrap(
    api.DELETE("/v1/mcp-servers/{id}", { params: { path: { id } } }),
    "Could not remove MCP server"
  )
}

export async function testMcpServer(id: string): Promise<McpConnectionTest> {
  const data = record(
    await unwrap(
      api.POST("/v1/mcp-servers/{id}/test", { params: { path: { id } } }),
      "Could not test MCP server"
    )
  )
  const result = record(data.test)
  const serverInfo = record(result.server_info)
  return {
    connected: result.connected === true,
    protocolVersion: stringValue(result.protocol_version),
    serverName: stringValue(serverInfo.name),
    serverVersion: stringValue(serverInfo.version),
    capabilities: record(result.capabilities),
  }
}

export async function configureMcpAuthorization(
  id: string,
  input: McpAuthorizationInput
): Promise<McpAuthorizationResult> {
  const data = record(
    await unwrap(
      api.PUT("/v1/mcp-servers/{id}/authorization", {
        params: { path: { id } },
        body: input,
      }),
      "Could not connect MCP server"
    )
  )
  const authorization = record(data.authorization)
  return {
    authorizationUrl: null,
    authStatus: authStatus(
      authorization.status ?? data.status,
      "oauth_authorization_code"
    ),
  }
}

export async function startMcpOAuth(
  id: string,
  principalType: "user" | "org_service" = "user",
  registration?: {
    clientId?: string
    clientSecret?: string
    scopes?: string[]
  }
): Promise<string> {
  const data = record(
    await unwrap(
      api.POST("/v1/mcp-servers/{id}/oauth/start", {
        params: { path: { id } },
        body: {
          principal_type: principalType,
          client_id: registration?.clientId,
          client_secret: registration?.clientSecret,
          scopes: registration?.scopes,
          redirect_after: "/w/settings/mcp",
        },
      }),
      "Could not start MCP sign in"
    )
  )
  const authorizationURL = stringValue(data.authorization_url)
  if (!authorizationURL)
    throw new Error("MCP server did not return a sign-in URL")
  return authorizationURL
}

export async function grantMcpServerToTeam(
  serverID: string,
  teamID: string
): Promise<void> {
  await unwrap(
    api.POST("/v1/orgs/current/teams/{teamID}/mcp-servers", {
      params: { path: { teamID } },
      body: { mcp_server_id: serverID },
    }),
    "Could not grant MCP server to team"
  )
}

export async function revokeMcpServerFromTeam(
  serverID: string,
  teamID: string
): Promise<void> {
  await unwrap(
    api.DELETE("/v1/orgs/current/teams/{teamID}/mcp-servers/{serverID}", {
      params: { path: { teamID, serverID } },
    }),
    "Could not revoke MCP server from team"
  )
}

export async function attachMcpServerToAgent(
  serverID: string,
  agentID: string,
  scope: McpServerScope
): Promise<void> {
  if (scope === "personal") {
    await unwrap(
      api.POST("/v1/agents/{agentID}/personal-mcp-servers", {
        params: { path: { agentID } },
        body: { mcp_server_id: serverID },
      }),
      "Could not attach MCP server to agent"
    )
    return
  }
  await unwrap(
    api.PUT("/v1/agents/{agentID}/mcp-servers", {
      params: { path: { agentID } },
      body: { mcp_server_id: serverID, state: "enabled" },
    }),
    "Could not attach MCP server to agent"
  )
}

export async function detachMcpServerFromAgent(
  serverID: string,
  agentID: string,
  scope: McpServerScope
): Promise<void> {
  if (scope === "personal") {
    await unwrap(
      api.DELETE("/v1/agents/{agentID}/personal-mcp-servers/{serverID}", {
        params: { path: { agentID, serverID } },
      }),
      "Could not detach MCP server from agent"
    )
    return
  }
  await unwrap(
    api.DELETE("/v1/agents/{agentID}/mcp-servers/{serverID}", {
      params: { path: { agentID, serverID } },
    }),
    "Could not detach MCP server from agent"
  )
}

function assignedServerIDs(value: unknown): string[] {
  const raw = record(value)
  const items = Array.isArray(value)
    ? value
    : Array.isArray(raw.mcp_servers)
      ? raw.mcp_servers
      : Array.isArray(raw.data)
        ? raw.data
        : []
  const ids: string[] = []
  for (const item of items) {
    if (typeof item === "string") {
      ids.push(item)
      continue
    }
    const row = record(item)
    const id = stringValue(row.mcp_server_id) || stringValue(row.id)
    if (id && row.state !== "disabled") ids.push(id)
  }
  return Array.from(new Set(ids))
}

export async function listTeamMcpServerIDs(
  teamID: string,
  signal?: AbortSignal
): Promise<string[]> {
  return assignedServerIDs(
    await unwrap(
      api.GET("/v1/orgs/current/teams/{teamID}/mcp-servers", {
        params: { path: { teamID } },
        signal,
      }),
      "Could not load team MCP access"
    )
  )
}

export async function listAgentMcpServerIDs(
  agentID: string,
  scope: McpServerScope,
  signal?: AbortSignal
): Promise<string[]> {
  if (scope === "personal") {
    return assignedServerIDs(
      await unwrap(
        api.GET("/v1/agents/{agentID}/personal-mcp-servers", {
          params: { path: { agentID } },
          signal,
        }),
        "Could not load agent MCP access"
      )
    )
  }
  return assignedServerIDs(
    await unwrap(
      api.GET("/v1/agents/{agentID}/mcp-servers", {
        params: { path: { agentID } },
        signal,
      }),
      "Could not load agent MCP access"
    )
  )
}
