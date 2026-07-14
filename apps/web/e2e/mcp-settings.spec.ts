import { expect, test, type Page } from "@playwright/test"

type MockRole = "admin" | "member"

type MockState = {
  servers: Array<Record<string, unknown>>
  requests: Array<{
    method: string
    path: string
    body: Record<string, unknown> | null
  }>
}

const PERSONAL_SERVER = {
  id: "personal-1",
  scope: "personal",
  owner_user_id: "user-1",
  name: "My Linear tools",
  slug: "my-linear-tools",
  url: "https://linear.example.test/mcp",
  transport: "streamable_http",
  auth_type: "static_bearer",
  authorization_policy: "user_required",
  status: "active",
  health_status: "unknown",
  tool_count: 12,
  user_authorization: {
    principal_type: "user",
    configured: true,
    status: "active",
  },
  // This intentionally simulates a buggy backend response. The page must not
  // render or retain it in its normalized display model.
  bearer_token: "sk-super-secret",
}

const ORG_SERVER = {
  id: "org-1",
  scope: "org",
  name: "Company GitHub tools",
  slug: "company-github-tools",
  url: "https://github.example.test/mcp",
  transport: "sse",
  auth_type: "oauth_authorization_code",
  authorization_policy: "user_required",
  status: "active",
  health_status: "unknown",
  user_authorization: {
    principal_type: "user",
    configured: false,
    status: "not_connected",
  },
}

async function openMcpSettings(page: Page, baseURL: string, role: MockRole) {
  await page.setViewportSize({ width: 1280, height: 1200 })
  const state: MockState = {
    servers: [{ ...PERSONAL_SERVER }, { ...ORG_SERVER }],
    requests: [],
  }
  await page.context().addCookies([
    {
      name: "__session",
      value: "mcp-settings-test-session",
      url: new URL("/", baseURL).toString(),
      httpOnly: true,
      sameSite: "Lax",
    },
  ])

  await page.route("**/api/proxy/**", async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/^\/api\/proxy/, "")
    const method = request.method()
    let body: Record<string, unknown> | null = null
    if (request.postData()) {
      body = request.postDataJSON() as Record<string, unknown>
    }
    state.requests.push({ method, path, body })

    const json = (value: unknown, status = 200) =>
      route.fulfill({ status, contentType: "application/json", json: value })

    if (method === "GET" && path === "/auth/me") {
      return json({
        user: { id: "user-1", email: "operator@example.com", name: "Operator" },
        orgs: [
          {
            id: "org-1",
            name: "Acme",
            role,
            onboarding_step: "complete",
          },
        ],
      })
    }
    if (method === "GET" && path === "/v1/plans") return json([])
    if (method === "GET" && path === "/v1/orgs/current/teams") {
      return json({
        data: [
          { id: "team-1", name: "Engineering" },
          { id: "team-2", name: "Support" },
        ],
      })
    }
    if (method === "GET" && path === "/v1/agents") {
      return json({
        data: [
          { id: "agent-1", name: "Kara", team_id: "team-1" },
          { id: "agent-2", name: "Scout", team_id: "team-2" },
        ],
      })
    }
    if (method === "GET" && path === "/v1/mcp-servers") {
      return json({ mcp_servers: state.servers })
    }
    if (method === "POST" && path === "/v1/mcp-servers") {
      const id = `created-${state.servers.length + 1}`
      const authorization = (body?.authorization ?? {}) as Record<
        string,
        unknown
      >
      const created: Record<string, unknown> = {
        id,
        ...(body ?? {}),
        slug: id,
        status: "active",
        health_status: "unknown",
        user_authorization:
          body?.scope === "personal" && body?.auth_type !== "none"
            ? {
                principal_type: "user",
                configured: true,
                status: "connected",
              }
            : undefined,
        service_authorization:
          body?.scope === "org" && body?.auth_type !== "none"
            ? {
                principal_type: "org_service",
                configured: true,
                status: "connected",
                client_id: authorization.client_id,
              }
            : undefined,
      }
      // The mock stores only the same redacted shape production returns.
      const { authorization: _secret, ...safeCreated } = created
      void _secret
      state.servers.push(safeCreated)
      return json({ mcp_server: created }, 201)
    }
    if (
      method === "GET" &&
      path === "/v1/agents/agent-1/personal-mcp-servers"
    ) {
      return json({ mcp_servers: [{ ...PERSONAL_SERVER }] })
    }
    if (
      method === "GET" &&
      path === "/v1/agents/agent-2/personal-mcp-servers"
    ) {
      return json({ mcp_servers: [] })
    }
    if (method === "GET" && path === "/v1/agents/agent-1/mcp-servers") {
      return json({ mcp_servers: [] })
    }
    if (method === "GET" && path === "/v1/agents/agent-2/mcp-servers") {
      return json({ mcp_servers: [] })
    }
    if (
      method === "GET" &&
      path === "/v1/orgs/current/teams/team-1/mcp-servers"
    ) {
      return json({ mcp_servers: [{ ...ORG_SERVER }] })
    }
    if (
      method === "GET" &&
      path === "/v1/orgs/current/teams/team-2/mcp-servers"
    ) {
      return json({ mcp_servers: [] })
    }
    if (
      method === "POST" &&
      path === "/v1/orgs/current/teams/team-2/mcp-servers"
    ) {
      return json({ status: "granted" }, 201)
    }
    if (method === "PUT" && path === "/v1/agents/agent-1/mcp-servers") {
      return json({ status: "enabled" })
    }
    if (
      method === "POST" &&
      path === "/v1/agents/agent-2/personal-mcp-servers"
    ) {
      return json({ status: "attached" }, 201)
    }
    if (
      method === "POST" &&
      path.startsWith("/v1/mcp-servers/") &&
      path.endsWith("/oauth/start")
    ) {
      return json({ authorization_url: `${baseURL}/oauth-test-target` })
    }
    if (method === "POST" && path.endsWith("/test")) {
      return json({
        test: {
          connected: true,
          protocol_version: "2025-11-25",
          server_info: { name: "Mock MCP", version: "1.0.0" },
          capabilities: { tools: {} },
        },
      })
    }

    return json({ error: `Unhandled mock route: ${method} ${path}` }, 404)
  })

  await page.goto("/w/settings/mcp")
  await expect(page.getByRole("heading", { name: "MCP servers" })).toBeVisible()
  return state
}

test.describe("MCP settings", () => {
  test("separates personal and organization servers without rendering secrets", async ({
    page,
    baseURL,
  }, testInfo) => {
    const state = await openMcpSettings(page, baseURL!, "admin")

    await expect(page.getByText("My Linear tools")).toBeVisible()
    await expect(page.getByText("Connected")).toBeVisible()
    await expect(page.getByText("Attached to 1 agent")).toBeVisible()
    await expect(page.getByText("sk-super-secret")).toHaveCount(0)

    await page.getByRole("tab", { name: /Organization/ }).click()
    await expect(page.getByText("Company GitHub tools")).toBeVisible()
    await expect(page.getByText("1 team")).toBeVisible()

    await page.getByRole("button", { name: "Manage" }).click()
    await expect(page.getByText("Legacy HTTP/SSE")).toBeVisible()
    await page.getByRole("button", { name: "Test connection" }).click()
    await expect(page.getByText("Healthy")).toBeVisible()
    await testInfo.attach("mcp-settings-organization-access", {
      body: await page.screenshot({ fullPage: true }),
      contentType: "image/png",
    })
    await page
      .getByRole("switch", { name: "Grant Support" })
      .click({ force: true })
    await expect
      .poll(() =>
        state.requests.some(
          (request) =>
            request.method === "POST" &&
            request.path === "/v1/orgs/current/teams/team-2/mcp-servers" &&
            request.body?.mcp_server_id === "org-1"
        )
      )
      .toBe(true)

    const karaSwitch = page.getByRole("switch", { name: "Grant Kara" })
    await expect(karaSwitch).toBeEnabled()
    await karaSwitch.click({ force: true })
    await expect
      .poll(() =>
        state.requests.some(
          (request) =>
            request.method === "PUT" &&
            request.path === "/v1/agents/agent-1/mcp-servers" &&
            request.body?.state === "enabled"
        )
      )
      .toBe(true)
  })

  test("creates personal and organization servers with scoped authorization", async ({
    page,
    baseURL,
  }) => {
    const state = await openMcpSettings(page, baseURL!, "admin")

    await page.getByRole("button", { name: "Add server" }).click()
    await page.locator('input[name="name"]').fill("Personal docs")
    await page
      .locator('input[name="url"]')
      .fill("https://docs.example.test/mcp")
    await page.getByRole("button", { name: "Add server" }).click()
    await expect(
      page.getByRole("heading", { name: "Personal docs", exact: true })
    ).toBeVisible()

    const personalCreate = state.requests.find(
      (request) =>
        request.method === "POST" && request.path === "/v1/mcp-servers"
    )
    expect(personalCreate?.body).toMatchObject({
      scope: "personal",
      name: "Personal docs",
      transport: "streamable_http",
      auth_type: "none",
      authorization_policy: "none",
    })

    await page.getByRole("tab", { name: /Organization/ }).click()
    await page.getByRole("button", { name: "Add server" }).click()
    await page.locator('input[name="name"]').fill("Shared support")
    await page
      .locator('input[name="url"]')
      .fill("https://support.example.test/mcp")
    await page.getByRole("button", { name: "Transport" }).click()
    await page.getByRole("option", { name: /Legacy HTTP\/SSE/ }).click()
    await page.getByRole("button", { name: "Authentication" }).click()
    await page.getByRole("option", { name: /Custom header/ }).click()
    await page.getByRole("button", { name: "Authorization ownership" }).click()
    await expect(
      page.getByRole("option", { name: /Each member uses their own account/ })
    ).toBeVisible()
    await page
      .getByRole("option", { name: /Organization service identity/ })
      .click()
    await page.locator('input[name="header-value"]').fill("header-super-secret")
    await page.getByRole("button", { name: "Add server" }).click()
    await expect(
      page.getByRole("heading", { name: "Shared support", exact: true })
    ).toBeVisible()
    await expect(page.getByText("header-super-secret")).toHaveCount(0)

    const orgCreate = state.requests.filter(
      (request) =>
        request.method === "POST" && request.path === "/v1/mcp-servers"
    )[1]
    expect(orgCreate?.body).toMatchObject({
      scope: "org",
      transport: "sse",
      auth_type: "static_header",
      authorization_policy: "service_required",
      header_name: "X-API-Key",
      authorization: {
        principal_type: "org_service",
        header_value: "header-super-secret",
      },
    })

    await page.getByRole("button", { name: "Add server" }).click()
    await page.locator('input[name="name"]').fill("Shared OAuth")
    await page
      .locator('input[name="url"]')
      .fill("https://oauth.example.test/mcp")
    await page.getByRole("button", { name: "Authentication" }).click()
    await page.getByRole("option", { name: /^OAuth Sign in/ }).click()
    await page.getByRole("button", { name: "Authorization ownership" }).click()
    await page
      .getByRole("option", { name: /Organization service identity/ })
      .click()
    await page.getByRole("button", { name: "Add server" }).click()
    await expect
      .poll(() =>
        state.requests.some(
          (request) =>
            request.method === "POST" &&
            request.path.endsWith("/oauth/start") &&
            request.path !== "/v1/mcp-servers/org-1/oauth/start" &&
            request.body?.principal_type === "org_service"
        )
      )
      .toBe(true)
  })

  test("shows members only their personal MCP management surface", async ({
    page,
    baseURL,
  }) => {
    await openMcpSettings(page, baseURL!, "member")

    await expect(page.getByRole("tab", { name: /Personal/ })).toHaveCount(0)
    await expect(page.getByRole("tab", { name: /Organization/ })).toHaveCount(0)
    await expect(page.getByText("My Linear tools")).toBeVisible()
    await expect(page.getByText("Company GitHub tools")).toHaveCount(0)
    await expect(page.getByRole("button", { name: "Add server" })).toBeVisible()
    await page.getByRole("button", { name: "Add server" }).click()
    await expect(
      page.getByRole("heading", { name: "Add personal server" })
    ).toBeVisible()
    await expect(page.getByText("Ownership")).toHaveCount(0)
    await expect(page.getByText("Company GitHub tools")).toHaveCount(0)
  })
})
