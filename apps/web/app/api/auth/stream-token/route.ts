import { NextRequest, NextResponse } from "next/server"
import * as Sentry from "@sentry/nextjs"
import { getSessionFromHeader, createSessionCookie } from "@/lib/auth/session"
import { refreshCoordinator, type RefreshOutcome } from "@/lib/auth/refresh"

const HIVY_API_URL = process.env.HIVY_API_URL ?? process.env.NEXT_PUBLIC_HIVY_API_URL as string

/**
 * Minimum remaining lifetime (ms) before we refresh. A live SSE stream needs
 * more headroom than an ordinary proxied request, so this is larger than the
 * proxy's reactive threshold — but the refresh itself runs through the shared
 * refreshCoordinator so it can never double-spend a single-use refresh token
 * that the proxy is rotating concurrently.
 */
const MIN_TTL = 5 * 60 * 1000 // 5 minutes

function captureRefreshFailure(
  stage: string,
  context: Record<string, unknown> = {}
) {
  Sentry.withScope((scope) => {
    scope.setTag("component", "web_stream_token")
    scope.setTag("refresh_stage", stage)
    scope.setExtras(context)
    Sentry.captureMessage("web stream token refresh failed", "warning")
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
      scope.setTag("component", "web_stream_token")
      scope.setTag("refresh_stage", "request_error")
      scope.setExtra("reason", "auth_refresh_fetch_failed")
      Sentry.captureException(err)
    })
    return { session: null, definitivelyRejected: false }
  }

  if (!res.ok) {
    captureRefreshFailure("request_rejected", {
      status: res.status,
      statusText: res.statusText,
    })
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

/**
 * GET /api/auth/stream-token
 *
 * Returns a short-lived access token + org ID for direct backend SSE connections.
 * Refreshes the session if the token is close to expiry and persists the new cookie.
 */
export async function GET(req: NextRequest) {
  const cookieHeader = req.headers.get("cookie")
  let session = await getSessionFromHeader(cookieHeader)

  if (!session) {
    return NextResponse.json({ error: "not_authenticated" }, { status: 401 })
  }

  let setCookie: string | null = null

  // Refresh if token doesn't have enough remaining lifetime for a stream.
  // Routed through the shared coordinator so a concurrent proxy refresh of the
  // same single-use token is reused rather than racing it.
  if (session.expires_at - Date.now() < MIN_TTL) {
    const outcome = await refreshCoordinator.refresh(
      session.refresh_token,
      refreshTokens
    )
    if (!outcome.session) {
      captureRefreshFailure("stream_token", {
        path: "/api/auth/stream-token",
        method: req.method,
      })
      return NextResponse.json({ error: "refresh_failed" }, { status: 401 })
    }
    session = outcome.session
    setCookie = await createSessionCookie(outcome.session)
  }

  const activeOrg = req.cookies.get("hivy_active_org")?.value ?? null

  const res = NextResponse.json({
    access_token: session.access_token,
    org_id: activeOrg,
    expires_at: session.expires_at,
  })

  if (setCookie) {
    res.headers.append("set-cookie", setCookie)
  }

  return res
}
