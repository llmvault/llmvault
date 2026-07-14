import { describe, expect, it } from "vitest"
import { normalizeMcpServer } from "./mcp-api"
import {
  assignmentSummary,
  authorizationPrincipal,
  canCreateMcpServer,
  serversForScope,
} from "./mcp-ui"

const baseInput = {
  name: "Linear",
  url: "https://mcp.example.test",
  scope: "personal" as const,
  authType: "none" as const,
  isAdmin: false,
  bearerToken: "",
  headerName: "",
  headerValue: "",
  clientId: "",
  clientSecret: "",
  tokenUrl: "",
}

describe("MCP settings state", () => {
  it("separates personal and organization servers", () => {
    const personal = normalizeMcpServer({
      id: "personal-1",
      name: "My Linear",
      url: "https://linear.example.test/mcp",
      scope: "personal",
    })
    const org = normalizeMcpServer({
      id: "org-1",
      name: "Company GitHub",
      url: "https://github.example.test/mcp",
      scope: "org",
      transport: "sse",
    })

    expect(serversForScope([personal, org], "personal")).toEqual([personal])
    expect(serversForScope([personal, org], "org")).toEqual([org])
    expect(org.transport).toBe("sse")
  })

  it("never projects plaintext credentials into the UI model", () => {
    const server = normalizeMcpServer({
      id: "server-1",
      name: "Private MCP",
      url: "https://private.example.test/mcp",
      auth_type: "static_header",
      auth_status: "connected",
      secret_set: true,
      header_name: "X-API-Key",
      header_value: "sk-super-secret",
      bearer_token: "also-secret",
      client_secret: "client-secret",
    })

    expect(server.secretSet).toBe(true)
    expect(JSON.stringify(server)).not.toContain("sk-super-secret")
    expect(JSON.stringify(server)).not.toContain("also-secret")
    expect(JSON.stringify(server)).not.toContain("client-secret")
  })

  it("requires admins for organization creation and credentials for protected auth", () => {
    expect(canCreateMcpServer(baseInput)).toBe(true)
    expect(
      canCreateMcpServer({ ...baseInput, scope: "org", isAdmin: false })
    ).toBe(false)
    expect(
      canCreateMcpServer({
        ...baseInput,
        scope: "org",
        isAdmin: true,
        authType: "static_bearer",
      })
    ).toBe(false)
    expect(
      canCreateMcpServer({
        ...baseInput,
        scope: "org",
        isAdmin: true,
        authType: "static_bearer",
        bearerToken: "token",
      })
    ).toBe(true)
    expect(
      canCreateMcpServer({
        ...baseInput,
        authType: "oauth_authorization_code",
      })
    ).toBe(true)
    expect(
      canCreateMcpServer({
        ...baseInput,
        authType: "oauth_authorization_code",
        clientId: "hivy-client",
      })
    ).toBe(true)
    expect(
      canCreateMcpServer({
        ...baseInput,
        authType: "oauth_client_credentials",
        clientId: "machine-client",
        clientSecret: "machine-secret",
        tokenUrl: "https://auth.example.test/token",
      })
    ).toBe(false)
  })

  it("describes personal and organization assignments without exposing ids", () => {
    expect(
      assignmentSummary({
        scope: "personal",
        teamIds: [],
        agentIds: ["agent-1"],
      })
    ).toBe("Attached to 1 agent")
    expect(
      assignmentSummary({
        scope: "org",
        teamIds: ["team-1", "team-2"],
        agentIds: ["agent-1"],
      })
    ).toBe("2 teams · 1 direct agent")
  })

  it("keeps authorization ownership independent from server ownership", () => {
    expect(authorizationPrincipal("org", "user_required")).toBe("user")
    expect(authorizationPrincipal("org", "service_required")).toBe(
      "org_service"
    )
    expect(authorizationPrincipal("personal", "service_required")).toBe("user")
  })
})
