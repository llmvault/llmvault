import { describe, it, expect, vi, beforeEach, afterEach } from "vitest"
import { NextRequest } from "next/server"

// Module-level mocks must be set up before the route module is loaded.

vi.mock("@sentry/nextjs", () => ({
  withScope: vi.fn(),
  captureException: vi.fn(),
  captureMessage: vi.fn(),
}))

vi.mock("@/lib/logger", () => ({
  log: {
    info: vi.fn(),
    debug: vi.fn(),
    warn: vi.fn(),
    error: vi.fn(),
    child: () => ({
      info: vi.fn(),
      debug: vi.fn(),
      warn: vi.fn(),
      error: vi.fn(),
    }),
  },
}))

vi.mock("@/lib/auth/session", () => ({
  getSessionFromHeader: vi.fn().mockResolvedValue(null),
  stripSessionCookie: vi.fn().mockReturnValue(""),
  createSessionCookie: vi.fn().mockResolvedValue("__session=mock; HttpOnly; Path=/"),
  clearSessionCookie: vi.fn().mockReturnValue("__session=; Max-Age=0"),
}))

vi.mock("@/lib/auth/refresh", () => ({
  refreshCoordinator: {
    refresh: vi.fn().mockResolvedValue({ session: null, definitivelyRejected: false }),
  },
}))

// Set HIVY_API_URL before the route module is loaded (it reads the env at module init time).
process.env.HIVY_API_URL = "http://backend-test"

const { POST } = await import("./route")

function makeRequest(path: string, options: { method?: string; body?: unknown } = {}) {
  const method = options.method ?? "GET"
  const url = `http://localhost/api/proxy/${path}`
  if (options.body !== undefined) {
    return new NextRequest(url, {
      method,
      body: JSON.stringify(options.body),
      headers: { "content-type": "application/json" },
    })
  }
  return new NextRequest(url, { method })
}

describe("Set-Cookie header forwarding", () => {
  const originalFetch = global.fetch

  afterEach(() => {
    global.fetch = originalFetch
  })

  it("forwards all upstream Set-Cookie headers without collapsing them", async () => {
    const upstreamHeaders = new Headers({
      "content-type": "application/json",
    })
    upstreamHeaders.append("set-cookie", "session_id=abc; HttpOnly; Path=/")
    upstreamHeaders.append("set-cookie", "theme=dark; Path=/; Max-Age=86400")

    const upstreamBody = JSON.stringify({ ok: true })
    global.fetch = vi.fn().mockResolvedValue(
      new Response(upstreamBody, {
        status: 200,
        headers: upstreamHeaders,
      })
    )

    const req = makeRequest("v1/some-endpoint", { method: "POST", body: {} })
    const res = await POST(req, { params: Promise.resolve({ path: ["v1", "some-endpoint"] }) })

    const setCookieValues = res.headers.getSetCookie()
    expect(setCookieValues.length).toBe(2)
    expect(setCookieValues.some((c) => c.includes("session_id=abc"))).toBe(true)
    expect(setCookieValues.some((c) => c.includes("theme=dark"))).toBe(true)
  })

  it("does not duplicate a single Set-Cookie header", async () => {
    const upstreamHeaders = new Headers({
      "content-type": "application/json",
      "set-cookie": "token=xyz; HttpOnly; Path=/",
    })

    global.fetch = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ data: 1 }), {
        status: 200,
        headers: upstreamHeaders,
      })
    )

    const req = makeRequest("v1/other", { method: "POST", body: {} })
    const res = await POST(req, { params: Promise.resolve({ path: ["v1", "other"] }) })

    const setCookieValues = res.headers.getSetCookie()
    expect(setCookieValues.length).toBe(1)
    expect(setCookieValues[0]).toContain("token=xyz")
  })
})

describe("AUTH_PATHS consumed-body fallthrough", () => {
  const originalFetch = global.fetch

  beforeEach(() => {
    process.env.HIVY_API_URL = "http://backend"
  })

  afterEach(() => {
    global.fetch = originalFetch
  })

  it("returns 200 JSON when an auth path responds without tokens (OTP challenge)", async () => {
    const challengeBody = { requires_otp: true, message: "Enter your OTP" }

    global.fetch = vi.fn().mockResolvedValue(
      new Response(JSON.stringify(challengeBody), {
        status: 200,
        headers: { "content-type": "application/json" },
      })
    )

    const req = makeRequest("auth/login", { method: "POST", body: { email: "a@b.com", password: "pass" } })
    const res = await POST(req, { params: Promise.resolve({ path: ["auth", "login"] }) })

    expect(res.status).toBe(200)
    const json = await res.json()
    expect(json.requires_otp).toBe(true)
    expect(json.message).toBe("Enter your OTP")
  })

  it("returns a non-200 auth response body correctly (e.g. 422 validation error)", async () => {
    // The token-interception block only runs when upstream.ok is true; auth
    // errors are passed through without modification.
    const errBody = { error: "invalid_credentials" }

    global.fetch = vi.fn().mockResolvedValue(
      new Response(JSON.stringify(errBody), {
        status: 401,
        headers: { "content-type": "application/json" },
      })
    )

    const req = makeRequest("auth/login", { method: "POST", body: { email: "x@y.com", password: "wrong" } })
    const res = await POST(req, { params: Promise.resolve({ path: ["auth", "login"] }) })

    expect(res.status).toBe(401)
    const json = await res.json()
    expect(json.error).toBe("invalid_credentials")
  })

  it("strips tokens from a successful login response and sets the session cookie", async () => {
    const successBody = {
      access_token: "at-secret",
      refresh_token: "rt-secret",
      expires_in: 900,
      user: { id: "u1" },
    }

    global.fetch = vi.fn().mockResolvedValue(
      new Response(JSON.stringify(successBody), {
        status: 200,
        headers: { "content-type": "application/json" },
      })
    )

    const req = makeRequest("auth/login", { method: "POST", body: { email: "a@b.com", password: "pass" } })
    const res = await POST(req, { params: Promise.resolve({ path: ["auth", "login"] }) })

    expect(res.status).toBe(200)
    const json = await res.json()
    expect(json.access_token).toBeUndefined()
    expect(json.refresh_token).toBeUndefined()
    expect(json.user).toEqual({ id: "u1" })
    const cookies = res.headers.getSetCookie()
    expect(cookies.some((c) => c.startsWith("__session=mock"))).toBe(true)
  })
})
