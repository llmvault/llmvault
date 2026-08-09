import { NextRequest, NextResponse } from "next/server"
import * as Sentry from "@sentry/nextjs"
import {
  getSessionFromHeader,
  stripSessionCookie,
  createSessionCookie,
  clearSessionCookie,
  type SessionData,
} from "@/lib/auth/session"
import { refreshCoordinator, type RefreshOutcome } from "@/lib/auth/refresh"
import { log } from "@/lib/logger"

// Server-to-API base URL (may be an internal address, e.g. http://api:8080 in a
// docker network). Distinct from the browser-facing public API URL that the
// client reads via clientConfig().apiUrl.
const HIVY_API_URL = process.env.HIVY_API_URL as string

log.info({ api_url: HIVY_API_URL }, "proxy route initialized")

// Paths whose successful responses contain tokens that should be persisted.
const AUTH_PATHS = new Set([
  "oauth/exchange",
  "auth/login",
  "auth/register",
  "auth/refresh",
  "auth/otp/verify",
])

const LOGOUT_PATH = "auth/logout"
const ORG_CURRENT_PATH = "v1/orgs/current"
const DIAGNOSTIC_BODY_MAX_CHARS = 512

function captureRefreshFailure(
  stage: string,
  context: Record<string, unknown> = {}
) {
  Sentry.withScope((scope) => {
    scope.setTag("component", "web_api_proxy")
    scope.setTag("refresh_stage", stage)

    if (typeof context.path === "string") scope.setTag("path", context.path)
    if (typeof context.method === "string") {
      scope.setTag("method", context.method)
    }

    scope.setExtras(context)
    Sentry.captureMessage("web access token refresh failed", "warning")
  })
}

async function refreshTokens(refreshToken: string): Promise<RefreshOutcome> {
  let res: Response
  try {
    res = await fetch(`${HIVY_API_URL}/auth/refresh`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refresh_token: refreshToken }),
    })
  } catch (err) {
    Sentry.withScope((scope) => {
      scope.setTag("component", "web_api_proxy")
      scope.setTag("refresh_stage", "request_error")
      scope.setExtra("reason", "auth_refresh_fetch_failed")
      Sentry.captureException(err)
    })
    // Transient (network) — do not force-logout.
    return { session: null, definitivelyRejected: false }
  }

  if (!res.ok) {
    captureRefreshFailure("request_rejected", {
      status: res.status,
      statusText: res.statusText,
    })
    // 401 means the refresh token is genuinely dead — the backend serves a
    // grace pair for concurrent rotations, so a 401 is authoritative. Other
    // statuses (5xx, etc.) are transient and must not force-logout.
    return { session: null, definitivelyRejected: res.status === 401 }
  }

  const data = await res.json()
  if (!data.access_token || !data.refresh_token) {
    captureRefreshFailure("invalid_payload", {
      status: res.status,
      hasAccessToken: Boolean(data.access_token),
      hasRefreshToken: Boolean(data.refresh_token),
    })
    return { session: null, definitivelyRejected: false }
  }

  return {
    session: {
      access_token: data.access_token,
      refresh_token: data.refresh_token,
      expires_at: Date.now() + (data.expires_in ?? 900) * 1000,
    },
    definitivelyRejected: false,
  }
}

// Dedupes concurrent refreshes by token hash so user B can never receive a
// session minted from user A's refresh token (see refreshCoordinator docs).
async function safeRefresh(refreshToken: string): Promise<RefreshOutcome> {
  return refreshCoordinator.refresh(refreshToken, refreshTokens)
}

function buildUpstreamHeaders(
  req: NextRequest,
  session: SessionData | null
): Headers {
  const headers = new Headers()

  // Forward safe headers (no authorization — we inject it from the cookie)
  for (const key of ["content-type", "accept"]) {
    const value = req.headers.get(key)
    if (value) headers.set(key, value)
  }

  const adminSecret = req.headers.get("x-hivy-admin-secret")
  if (adminSecret) headers.set("X-Hivy-Admin-Secret", adminSecret)

  const rawCookies = req.headers.get("cookie")
  if (rawCookies) {
    const cleaned = stripSessionCookie(rawCookies)
    if (cleaned) headers.set("cookie", cleaned)
  }

  if (session) {
    headers.set("authorization", `Bearer ${session.access_token}`)
  }

  const activeOrgCookie = req.cookies.get("hivy_active_org")
  if (activeOrgCookie?.value) {
    headers.set("X-Org-ID", activeOrgCookie.value)
  }

  return headers
}

async function forward(
  url: URL,
  method: string,
  headers: Headers,
  body: ArrayBuffer | undefined
) {
  return fetch(url, { method, headers, body })
}

async function handler(
  req: NextRequest,
  { params }: { params: Promise<{ path: string[] }> }
) {
  const { path } = await params
  const apiPath = path.join("/")
  const reqLog = log.child({ method: req.method, path: apiPath })

  reqLog.info("proxy request started")

  const url = new URL(`${HIVY_API_URL}/${apiPath}`)
  req.nextUrl.searchParams.forEach((value, key) =>
    url.searchParams.append(key, value)
  )

  reqLog.debug({ upstream_url: url.toString() }, "upstream url resolved")

  const rawCookies = req.headers.get("cookie")
  let session = await getSessionFromHeader(rawCookies)
  reqLog.debug({ has_session: !!session }, "session check")

  const body =
    req.method !== "GET" && req.method !== "HEAD"
      ? await req.arrayBuffer()
      : undefined

  let upstreamBody: ArrayBuffer | undefined = body
  const isLogout = apiPath === LOGOUT_PATH && req.method === "POST"

  if (isLogout && session) {
    const payload = body ? JSON.parse(new TextDecoder().decode(body)) : {}
    payload.refresh_token = session.refresh_token
    upstreamBody = new TextEncoder().encode(JSON.stringify(payload))
      .buffer as ArrayBuffer
  }

  if (
    session &&
    !AUTH_PATHS.has(apiPath) &&
    session.expires_at - Date.now() < 60_000
  ) {
    const refreshed = await safeRefresh(session.refresh_token)
    if (refreshed.session) {
      session = refreshed.session
    } else {
      captureRefreshFailure("proactive", {
        path: apiPath,
        method: req.method,
      })
    }
  }

  const isOrgCurrent = apiPath === ORG_CURRENT_PATH
  if (isOrgCurrent) {
    reqLog.info(
      {
        upstream_origin: url.origin,
        upstream_path: url.pathname,
        has_session: Boolean(session),
        has_active_org: Boolean(req.cookies.get("hivy_active_org")?.value),
      },
      "org current proxy request"
    )
  }

  const headers = buildUpstreamHeaders(req, session)
  let upstream: Response
  try {
    upstream = await forward(url, req.method, headers, upstreamBody)
    reqLog.info({ status: upstream.status }, "upstream response received")
  } catch (err) {
    reqLog.error({ err, upstream_url: url.toString() }, "upstream fetch failed")
    return NextResponse.json({ error: "upstream_unavailable" }, { status: 502 })
  }

  let refreshedSession: SessionData | null = null
  let refreshDefinitivelyRejected = false

  if (
    upstream.status === 401 &&
    session &&
    !AUTH_PATHS.has(apiPath) &&
    !isLogout
  ) {
    reqLog.info("got 401, attempting token refresh")
    const outcome = await safeRefresh(session.refresh_token)
    if (outcome.session) {
      reqLog.info("token refresh succeeded, retrying request")
      refreshedSession = outcome.session
      session = outcome.session
      const retryHeaders = buildUpstreamHeaders(req, outcome.session)
      upstream = await forward(url, req.method, retryHeaders, body)
      reqLog.info({ status: upstream.status }, "retry response received")
    } else {
      refreshDefinitivelyRejected = outcome.definitivelyRejected
      reqLog.warn("token refresh failed")
      captureRefreshFailure("retry_after_401", {
        path: apiPath,
        method: req.method,
        upstreamStatus: upstream.status,
      })
    }
  }

  if (isOrgCurrent) {
    const diagnostics = {
      status: upstream.status,
      status_text: upstream.statusText,
      content_type: upstream.headers.get("content-type") ?? undefined,
      server: upstream.headers.get("server") ?? undefined,
      response_body: undefined as string | undefined,
    }

    if (upstream.status === 404) {
      try {
        diagnostics.response_body = (await upstream.clone().text()).slice(
          0,
          DIAGNOSTIC_BODY_MAX_CHARS
        )
      } catch (err) {
        reqLog.warn({ err }, "org current response body could not be read")
      }
    }

    if (upstream.status === 404) {
      reqLog.warn(diagnostics, "org current upstream response")
    } else {
      reqLog.info(diagnostics, "org current upstream response")
    }
  }

  const responseHeaders = new Headers()
  const skipHeaders = new Set([
    "transfer-encoding",
    "content-encoding",
    "content-length",
    "set-cookie",
  ])
  upstream.headers.forEach((value, key) => {
    if (skipHeaders.has(key.toLowerCase())) return
    responseHeaders.set(key, value)
  })
  // Preserve each Set-Cookie header individually — Headers.set collapses
  // multiple values into one comma-joined string, which breaks cookie parsing.
  for (const cookie of upstream.headers.getSetCookie()) {
    responseHeaders.append("set-cookie", cookie)
  }

  if (AUTH_PATHS.has(apiPath) && upstream.ok) {
    reqLog.info("intercepting auth response")
    try {
      const data = await upstream.json()
      reqLog.debug(
        {
          has_access_token: !!data.access_token,
          has_refresh_token: !!data.refresh_token,
        },
        "auth response parsed"
      )

      if (data.access_token && data.refresh_token) {
        const newSession: SessionData = {
          access_token: data.access_token,
          refresh_token: data.refresh_token,
          expires_at: Date.now() + (data.expires_in ?? 900) * 1000,
        }
        const cookie = await createSessionCookie(newSession)
        reqLog.debug({ cookie_length: cookie.length }, "session cookie created")

        // Build clean response headers — don't carry over content-length
        // from the upstream response since we're returning a different body
        const authHeaders = new Headers()
        authHeaders.append("set-cookie", cookie)

        // Strip tokens from what the client receives
        const {
          access_token: _a,
          refresh_token: _r,
          expires_in: _e,
          ...safe
        } = data
        reqLog.info(
          { response_keys: Object.keys(safe), status: upstream.status },
          "auth response complete, session cookie set"
        )
        return NextResponse.json(safe, {
          status: upstream.status,
          headers: authHeaders,
        })
      }

      // Tokens absent in an auth response (e.g. OTP challenge, error details).
      // The body was already consumed via upstream.json() above, so we must
      // re-serialize rather than falling through to `upstream.body` (which is
      // now disturbed and would throw a 500).
      reqLog.info(
        { status: upstream.status },
        "auth path without tokens, re-serializing body"
      )
      return NextResponse.json(data, {
        status: upstream.status,
        headers: responseHeaders,
      })
    } catch (err) {
      reqLog.error({ err }, "auth response interception failed")
      return NextResponse.json(
        { error: "session_creation_failed" },
        { status: 502 }
      )
    }
  }

  if (refreshedSession) {
    responseHeaders.append(
      "set-cookie",
      await createSessionCookie(refreshedSession)
    )
  }

  if (!AUTH_PATHS.has(apiPath) && !refreshedSession && session && rawCookies) {
    const original = await getSessionFromHeader(rawCookies)
    if (original && original.access_token !== session.access_token) {
      responseHeaders.append("set-cookie", await createSessionCookie(session))
    }
  }

  // A stale cookie may be undecryptable after the local session secret changes.
  // Explicit logout must still recover the browser even when upstream cannot
  // revoke a token it was unable to read.
  if (isLogout) {
    responseHeaders.append("set-cookie", clearSessionCookie())
  }

  // Clear the session only when the refresh token was *definitively* rejected
  // (backend 401 — dead even after the grace window). A transient refresh
  // failure or a lost concurrent-rotation race must not force-logout the user;
  // the backend grace window + shared single-flight let the next request
  // recover instead.
  if (
    upstream.status === 401 &&
    session &&
    !refreshedSession &&
    !AUTH_PATHS.has(apiPath) &&
    refreshDefinitivelyRejected
  ) {
    responseHeaders.append("set-cookie", clearSessionCookie())
  }

  reqLog.info({ status: upstream.status }, "proxy response complete")
  return new NextResponse(upstream.body, {
    status: upstream.status,
    statusText: upstream.statusText,
    headers: responseHeaders,
  })
}

export const GET = handler
export const POST = handler
export const PUT = handler
export const PATCH = handler
export const DELETE = handler
