"use client"

import { api } from "@/lib/api/client"
import type { components } from "@/lib/api/schema"

const ACCESS_EXPIRY_BUFFER_MS = 5 * 60 * 1000

type RawSessionSandboxAccess =
  components["schemas"]["sessionSandboxAccessResponse"]

export type SessionSandboxAccess = RawSessionSandboxAccess & {
  session_id: string
  sandbox_id: string
  sandbox_base_url: string
  token: string
  expires_at: string
}

interface CachedAccess {
  access: SessionSandboxAccess
}

interface SandboxAccessOptions {
  expectedSandboxId?: string | null
  force?: boolean
}

const accessBySessionId = new Map<string, CachedAccess>()
const pendingBySessionId = new Map<string, Promise<SessionSandboxAccess>>()

export function getCachedSessionSandboxAccess(
  sessionId: string,
  options: Pick<SandboxAccessOptions, "expectedSandboxId"> = {}
) {
  const cached = accessBySessionId.get(sessionId)
  if (!cached) return null
  if (!accessMatchesExpectedSandbox(cached.access, options.expectedSandboxId)) {
    accessBySessionId.delete(sessionId)
    return null
  }
  if (!isSessionSandboxAccessFresh(cached.access)) {
    accessBySessionId.delete(sessionId)
    return null
  }
  return cached.access
}

export async function getSessionSandboxAccess(
  sessionId: string,
  options: SandboxAccessOptions = {}
) {
  if (!options.force) {
    const cached = getCachedSessionSandboxAccess(sessionId, options)
    if (cached) return cached
  }

  const pending = pendingBySessionId.get(sessionId)
  if (pending) return pending

  const request = requestFreshSandboxAccess(sessionId)
  pendingBySessionId.set(sessionId, request)
  try {
    const access = await request
    accessBySessionId.set(sessionId, {
      access,
    })
    return access
  } finally {
    pendingBySessionId.delete(sessionId)
  }
}

export function clearSessionSandboxAccess(sessionId?: string) {
  if (sessionId) {
    accessBySessionId.delete(sessionId)
    pendingBySessionId.delete(sessionId)
    return
  }
  accessBySessionId.clear()
  pendingBySessionId.clear()
}

export function isSessionSandboxAccessFresh(
  access: Pick<SessionSandboxAccess, "expires_at">,
  nowMs = Date.now()
) {
  const expiresAtMs = Date.parse(access.expires_at)
  return (
    Number.isFinite(expiresAtMs) &&
    expiresAtMs - ACCESS_EXPIRY_BUFFER_MS > nowMs
  )
}

function accessMatchesExpectedSandbox(
  access: SessionSandboxAccess,
  expectedSandboxId?: string | null
) {
  return !expectedSandboxId || access.sandbox_id === expectedSandboxId
}

async function requestFreshSandboxAccess(sessionId: string) {
  const { data, error } = await api.POST("/v1/sessions/{id}/sandbox-access", {
    params: { path: { id: sessionId } },
  })
  if (error || !data) {
    throw new Error(sandboxAccessErrorMessage(error))
  }
  return requireSandboxAccess(data)
}

function requireSandboxAccess(
  data: RawSessionSandboxAccess
): SessionSandboxAccess {
  if (
    !data.session_id ||
    !data.sandbox_id ||
    !data.sandbox_base_url ||
    !data.token ||
    !data.expires_at ||
    !isSessionSandboxAccessFresh({
      expires_at: data.expires_at,
    })
  ) {
    throw new Error("Sandbox access is not available.")
  }
  return data as SessionSandboxAccess
}

function sandboxAccessErrorMessage(error: unknown) {
  return typeof error === "object" &&
    error &&
    "error" in error &&
    typeof error.error === "string"
    ? error.error
    : "Sandbox access is not available."
}
