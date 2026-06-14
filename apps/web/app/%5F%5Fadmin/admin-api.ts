import type {
  AdminIntegrationDefinition,
  LLMProvider,
  PageResponse,
  SystemCredential,
} from "./types"

export type AdminData = {
  integrations: AdminIntegrationDefinition[]
  credentials: SystemCredential[]
  providers: LLMProvider[]
}

export const adminDataQueryKey = (version: number) =>
  ["admin", "setup", version] as const

export async function loadAdminData(secret: string): Promise<AdminData> {
  const [integrations, credentialPage, providers] = await Promise.all([
    adminFetch<AdminIntegrationDefinition[]>("/v1/admin/integrations", secret),
    adminFetch<PageResponse<SystemCredential>>(
      "/v1/admin/system-credentials?limit=100",
      secret
    ),
    adminFetch<LLMProvider[]>("/v1/admin/llm-providers", secret),
  ])
  return {
    integrations,
    credentials: credentialPage.data ?? [],
    providers,
  }
}

export function createSystemCredential(secret: string, body: unknown) {
  return adminFetch<SystemCredential>("/v1/admin/system-credentials", secret, {
    method: "POST",
    body: JSON.stringify(body),
  })
}

export function revokeSystemCredential(secret: string, id: string) {
  return adminFetch(`/v1/admin/system-credentials/${id}`, secret, {
    method: "DELETE",
  })
}

export function upsertAdminIntegration(
  secret: string,
  id: string,
  credentials?: Record<string, string>
) {
  return adminFetch(`/v1/admin/integrations/${id}`, secret, {
    method: "PUT",
    body: JSON.stringify({ credentials }),
  })
}

export function deleteAdminIntegration(secret: string, id: string) {
  return adminFetch(`/v1/admin/integrations/${id}`, secret, {
    method: "DELETE",
  })
}

export async function adminFetch<T = unknown>(
  path: string,
  secret: string,
  init: RequestInit = {}
): Promise<T> {
  const headers = new Headers(init.headers)
  headers.set("X-Hivy-Admin-Secret", secret)
  if (init.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json")
  }

  const response = await fetch(`/api/proxy${path}`, {
    ...init,
    headers,
  })
  const text = await response.text()
  const payload = text ? parseJSON(text) : null

  if (!response.ok) {
    const message =
      payload && typeof payload === "object" && "error" in payload
        ? String((payload as { error: unknown }).error)
        : `Request failed with status ${response.status}`
    throw new Error(message)
  }
  return payload as T
}

export function errorMessage(error: unknown, fallback: string) {
  return error instanceof Error && error.message ? error.message : fallback
}

function parseJSON(text: string): unknown {
  try {
    return JSON.parse(text)
  } catch {
    return text
  }
}
