// Typed fetch helpers for the app's own backend. The SPA only ever calls
// /api/* on its own origin — never the Hivy API or third parties directly.

export type Me = {
  user_id: string
  user_name: string
  user_email: string
  user_avatar: string
  org_id: string
  org_name: string
  role: string
  expires_at: number
}

export type SheetField = {
  id: string
  name: string
  type: string
  options: Record<string, unknown>
  position: number
}

export type SheetPage = {
  id: string
  sheet_id: string
  name: string
  position: number
  display_field_id?: string
  fields: SheetField[]
  row_count: number
}

export type SheetStructure = {
  sheet: {
    id: string
    slug: string
    name: string
    description: string
    icon: string
  }
  pages: SheetPage[]
}

export type ApiFetchOptions = {
  /** HTTP method, default GET. */
  method?: string
  /** JSON-encoded into the request body when present. */
  body?: unknown
}

/**
 * Call a JSON endpoint on the app backend. A 401 means the cookie session
 * expired; the backend includes launch_url in the body. The app runs inside
 * an iframe on the Hivy frontend, so re-auth means sending the TOP window
 * back through the Hivy launch flow — never navigating the iframe itself.
 *
 * All data access goes through this (via the query hooks in src/hooks/), so
 * the 401 handling above applies to every request the SPA makes.
 */
export async function apiFetch<T>(
  path: string,
  opts: ApiFetchOptions = {}
): Promise<T> {
  const res = await fetch(path, {
    method: opts.method ?? "GET",
    headers: {
      Accept: "application/json",
      ...(opts.body !== undefined
        ? { "Content-Type": "application/json" }
        : {}),
    },
    body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
  })
  if (res.status === 401) {
    const body = await res.json().catch(() => null)
    if (body?.launch_url) {
      // Cross-origin frames may set top.location (navigation is allowed even
      // when reading it is not); fall back to our own window outside a frame.
      ;(window.top ?? window).location.href = body.launch_url
    }
    throw new Error("session expired")
  }
  if (!res.ok) {
    const body = await res.json().catch(() => null)
    throw new Error(body?.error ?? `request failed with status ${res.status}`)
  }
  return res.json() as Promise<T>
}

/** GET a JSON endpoint on the app backend. See apiFetch for 401 semantics. */
export async function apiGet<T>(path: string): Promise<T> {
  return apiFetch<T>(path)
}
