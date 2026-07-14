import type {
  McpAuthStatus,
  McpAuthType,
  McpAuthorizationPolicy,
  McpHealthStatus,
  McpServer,
  McpServerScope,
  McpTransport,
} from "./mcp-api"

export const TRANSPORT_OPTIONS: ReadonlyArray<{
  id: McpTransport
  label: string
  description: string
}> = [
  {
    id: "streamable_http",
    label: "Streamable HTTP",
    description: "Recommended for current MCP servers.",
  },
  {
    id: "sse",
    label: "Legacy HTTP/SSE",
    description: "Compatibility mode for older MCP servers.",
  },
]

export function transportLabel(transport: McpTransport): string {
  return transport === "sse" ? "Legacy HTTP/SSE" : "Streamable HTTP"
}

export const AUTH_OPTIONS: ReadonlyArray<{
  id: McpAuthType
  label: string
  description: string
}> = [
  {
    id: "none",
    label: "No authentication",
    description: "For public or network-protected servers.",
  },
  {
    id: "oauth_authorization_code",
    label: "OAuth",
    description: "Sign in with your account after adding the server.",
  },
  {
    id: "static_bearer",
    label: "Bearer token",
    description: "Send a private token in the Authorization header.",
  },
  {
    id: "static_header",
    label: "Custom header",
    description: "Send an API key using a header name you choose.",
  },
  {
    id: "oauth_client_credentials",
    label: "OAuth client credentials",
    description: "Use a machine identity for scheduled work.",
  },
]

export const AUTHORIZATION_POLICY_OPTIONS: ReadonlyArray<{
  id: "user_required" | "service_required"
  label: string
  description: string
}> = [
  {
    id: "user_required",
    label: "Each member uses their own account",
    description: "Actions use the current member’s credential and permissions.",
  },
  {
    id: "service_required",
    label: "Organization service identity",
    description: "Authorized agents share one credential managed by admins.",
  },
]

export function defaultAuthorizationPolicy(
  scope: McpServerScope,
  authType: McpAuthType
): McpAuthorizationPolicy {
  if (authType === "none") return "none"
  if (scope === "personal") return "user_required"
  if (authType === "oauth_authorization_code") return "user_required"
  return "service_required"
}

export function authorizationPrincipal(
  scope: McpServerScope,
  policy: McpAuthorizationPolicy
): "user" | "org_service" {
  if (scope === "personal" || policy === "user_required") return "user"
  return "org_service"
}

export function authorizationPolicyLabel(
  policy: McpAuthorizationPolicy
): string {
  switch (policy) {
    case "user_required":
      return "Per-member identity"
    case "service_required":
      return "Organization identity"
    case "prefer_user":
      return "Prefer member identity"
    case "prefer_service":
      return "Prefer organization identity"
    default:
      return "No authentication"
  }
}

export function serversForScope(
  servers: readonly McpServer[],
  scope: McpServerScope
): McpServer[] {
  return servers.filter((server) => server.scope === scope)
}

export function safeServerHost(url: string): string {
  try {
    return new URL(url).host
  } catch {
    return url || "Endpoint unavailable"
  }
}

function isValidMcpURL(value: string): boolean {
  try {
    const url = new URL(value)
    return url.protocol === "https:" || url.protocol === "http:"
  } catch {
    return false
  }
}

export function canCreateMcpServer(input: {
  name: string
  url: string
  scope: McpServerScope
  authType: McpAuthType
  isAdmin: boolean
  bearerToken: string
  headerName: string
  headerValue: string
  clientId: string
  clientSecret: string
  tokenUrl: string
}): boolean {
  if (!input.name.trim() || !isValidMcpURL(input.url.trim())) return false
  if (input.scope === "org" && !input.isAdmin) return false
  if (
    input.scope === "personal" &&
    input.authType === "oauth_client_credentials"
  ) {
    return false
  }
  if (input.authType === "static_bearer") return input.bearerToken.trim() !== ""
  if (input.authType === "static_header") {
    return input.headerName.trim() !== "" && input.headerValue.trim() !== ""
  }
  if (input.authType === "oauth_authorization_code") {
    return true
  }
  if (input.authType === "oauth_client_credentials") {
    return (
      input.clientId.trim() !== "" &&
      input.clientSecret.trim() !== "" &&
      isValidMcpURL(input.tokenUrl.trim())
    )
  }
  return true
}

export function authLabel(
  type: McpAuthType,
  status: McpAuthStatus,
  secretSet: boolean
): string {
  if (type === "none") return "No authentication"
  if (status === "connected") return "Connected"
  if (status === "pending") return "Waiting for sign in"
  if (status === "expired") return "Reconnect required"
  if (status === "error") return "Authentication error"
  if (secretSet) return "Credential saved"
  return "Not connected"
}

export function healthLabel(status: McpHealthStatus): string {
  switch (status) {
    case "healthy":
      return "Healthy"
    case "degraded":
      return "Degraded"
    case "unhealthy":
      return "Unavailable"
    case "checking":
      return "Checking"
    default:
      return "Not checked"
  }
}

export function assignmentSummary(
  server: Pick<McpServer, "scope" | "teamIds" | "agentIds">
): string {
  if (server.scope === "personal") {
    if (server.agentIds.length === 0) return "Not attached to any agents"
    return server.agentIds.length === 1
      ? "Attached to 1 agent"
      : `Attached to ${server.agentIds.length} agents`
  }
  const parts: string[] = []
  if (server.teamIds.length > 0) {
    parts.push(
      server.teamIds.length === 1 ? "1 team" : `${server.teamIds.length} teams`
    )
  }
  if (server.agentIds.length > 0) {
    parts.push(
      server.agentIds.length === 1
        ? "1 direct agent"
        : `${server.agentIds.length} direct agents`
    )
  }
  return parts.length > 0 ? parts.join(" · ") : "No access granted"
}
