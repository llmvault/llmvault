export type CanvasArtifactType = "web_page" | "presentation" | string
export type CanvasViewportMode = "desktop" | "tablet" | "mobile"

export interface CanvasArtifactProject {
  id: string
  slug?: string
  name: string
  description?: string
  artifactCount?: number
  updatedAt?: string
}

export interface CanvasArtifact {
  id: string
  projectId?: string
  slug?: string
  name: string
  type: CanvasArtifactType
  entryFile?: string
  updatedAt?: string
}

export interface CanvasArtifactPreviewURL {
  url: string
  expiresAt?: number
  expiresIn?: number
}

export interface CanvasArtifactCommentPayload {
  artifact_id: string
  artifact_name: string
  artifact_slug?: string
  artifact_type: CanvasArtifactType
  project_id?: string
  project_name?: string
  project_slug?: string
  selector?: string
  viewport: CanvasViewportMode
  preview_url?: string
  body: string
  created_at: string
}

export async function fetchCanvasProjects(
  signal?: AbortSignal
): Promise<CanvasArtifactProject[]> {
  const data = await requestJSON<unknown>("/v1/canvas/projects", { signal })
  return normalizeCanvasProjectList(data)
}

export async function fetchCanvasArtifacts(
  input: { projectId?: string | null; sessionId?: string | null },
  signal?: AbortSignal
): Promise<CanvasArtifact[]> {
  const params: Record<string, string> = {}
  if (input.projectId) params.project_id = input.projectId
  if (input.sessionId) params.session_id = input.sessionId

  const data = await requestJSON<unknown>("/v1/canvas/artifacts", {
    params,
    signal,
  })
  return normalizeCanvasArtifactList(data)
}

export async function fetchCanvasArtifact(
  artifactId: string,
  signal?: AbortSignal
): Promise<CanvasArtifact> {
  const data = await requestJSON<unknown>(
    `/v1/canvas/artifacts/${encodeURIComponent(artifactId)}`,
    { signal }
  )
  const artifact = normalizeCanvasArtifact(unwrapRecord(data, "artifact"))
  if (!artifact) throw new Error("Canvas artifact response was incomplete.")
  return artifact
}

export async function fetchCanvasArtifactPreviewURL(
  input: { artifactId: string; sessionId?: string | null },
  signal?: AbortSignal
): Promise<CanvasArtifactPreviewURL> {
  const params = input.sessionId ? { session_id: input.sessionId } : undefined
  const data = await requestJSON<unknown>(
    `/v1/canvas/artifacts/${encodeURIComponent(input.artifactId)}/preview-url`,
    {
      method: "POST",
      params,
      body: input.sessionId ? { session_id: input.sessionId } : {},
      signal,
    }
  )
  const preview = normalizeCanvasPreviewURL(data)
  if (!preview) throw new Error("Canvas preview response was incomplete.")
  return preview
}

export async function sendCanvasArtifactComment(
  sessionId: string,
  comment: CanvasArtifactCommentPayload,
  signal?: AbortSignal
): Promise<unknown> {
  return requestJSON<unknown>(
    `/v1/sessions/${encodeURIComponent(sessionId)}/messages`,
    {
      method: "POST",
      body: {
        text: comment.body,
        artifact_comments: [comment],
      },
      signal,
    }
  )
}

export function canvasArtifactCommentPayload(input: {
  artifact: CanvasArtifact
  project?: CanvasArtifactProject | null
  viewport: CanvasViewportMode
  body: string
  selector?: string
  previewUrl?: string
  now?: Date
}): CanvasArtifactCommentPayload {
  return {
    artifact_id: input.artifact.id,
    artifact_name: input.artifact.name,
    artifact_slug: input.artifact.slug,
    artifact_type: input.artifact.type,
    project_id: input.project?.id ?? input.artifact.projectId,
    project_name: input.project?.name,
    project_slug: input.project?.slug,
    selector: input.selector || undefined,
    viewport: input.viewport,
    preview_url: input.previewUrl,
    body: input.body,
    created_at: (input.now ?? new Date()).toISOString(),
  }
}

export function normalizeCanvasProjectList(
  value: unknown
): CanvasArtifactProject[] {
  const list = Array.isArray(value)
    ? value
    : arrayValue(recordValue(value)?.projects)
  return list.map(normalizeCanvasProject).filter(isPresent)
}

export function normalizeCanvasArtifactList(value: unknown): CanvasArtifact[] {
  const list = Array.isArray(value)
    ? value
    : arrayValue(recordValue(value)?.artifacts)
  return list.map(normalizeCanvasArtifact).filter(isPresent)
}

export function normalizeCanvasProject(
  value: unknown
): CanvasArtifactProject | null {
  const record = recordValue(value)
  if (!record) return null

  const id = stringValue(record.id, record.project_id, record.projectId)
  if (!id) return null

  return {
    id,
    slug: stringValue(record.slug),
    name: stringValue(record.name) || "Untitled project",
    description: stringValue(record.description),
    artifactCount: numberValue(
      record.artifact_count,
      record.artifacts_count,
      record.artifactCount,
      arrayValue(record.artifacts)?.length
    ),
    updatedAt: stringValue(record.updated_at, record.updatedAt),
  }
}

export function normalizeCanvasArtifact(value: unknown): CanvasArtifact | null {
  const record = recordValue(value)
  if (!record) return null

  const id = stringValue(record.id, record.artifact_id, record.artifactId)
  if (!id) return null

  const manifest = recordValue(record.manifest)
  return {
    id,
    projectId: stringValue(
      record.project_id,
      record.projectId,
      record.canvas_project_id
    ),
    slug: stringValue(record.slug),
    name: stringValue(record.name) || "Untitled artifact",
    type:
      stringValue(record.type, record.artifact_type, record.artifactType) ||
      "web_page",
    entryFile: stringValue(
      record.entry_file,
      record.entryFile,
      manifest?.entry_file,
      manifest?.entryFile
    ),
    updatedAt: stringValue(record.updated_at, record.updatedAt),
  }
}

function normalizeCanvasPreviewURL(
  value: unknown
): CanvasArtifactPreviewURL | null {
  const record = recordValue(value)
  if (!record) return null

  const url = stringValue(record.url, record.preview_url, record.previewUrl)
  if (!url) return null

  const expiresIn = numberValue(record.expires_in, record.expiresIn)
  const expiresAt = numberValue(record.expires_at, record.expiresAt)
  return {
    url,
    expiresIn,
    expiresAt:
      expiresAt ??
      (expiresIn === undefined ? undefined : Date.now() + expiresIn * 1000),
  }
}

async function requestJSON<T>(
  path: string,
  options: {
    method?: string
    params?: Record<string, string>
    body?: unknown
    signal?: AbortSignal
  } = {}
): Promise<T> {
  const response = await fetch(apiProxyPath(path, options.params), {
    method: options.method ?? "GET",
    headers: {
      Accept: "application/json",
      ...(options.body === undefined
        ? {}
        : { "Content-Type": "application/json" }),
    },
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
    signal: options.signal,
  })

  if (!response.ok) throw await requestError(response)
  return (await response.json()) as T
}

function apiProxyPath(path: string, params?: Record<string, string>): string {
  const normalizedPath = path.startsWith("/") ? path : `/${path}`
  const url = new URL(`/api/proxy${normalizedPath}`, "http://localhost")
  for (const [key, value] of Object.entries(params ?? {})) {
    if (value) url.searchParams.set(key, value)
  }
  return `${url.pathname}${url.search}`
}

async function requestError(response: Response): Promise<Error> {
  let message = `Request failed with ${response.status}`
  try {
    const payload = recordValue(await response.json())
    message =
      stringValue(payload?.message, payload?.error, payload?.detail) || message
  } catch {
    const text = await response.text().catch(() => "")
    if (text.trim()) message = text.trim()
  }
  return new Error(message)
}

function unwrapRecord(value: unknown, key: string): unknown {
  const record = recordValue(value)
  return record?.[key] ?? value
}

function recordValue(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) return null
  return value as Record<string, unknown>
}

function arrayValue(value: unknown): unknown[] {
  return Array.isArray(value) ? value : []
}

function stringValue(...values: unknown[]): string | undefined {
  for (const value of values) {
    if (typeof value === "string" && value.trim()) return value.trim()
  }
  return undefined
}

function numberValue(...values: unknown[]): number | undefined {
  for (const value of values) {
    if (typeof value === "number" && Number.isFinite(value)) return value
  }
  return undefined
}

function isPresent<T>(value: T | null | undefined): value is T {
  return value !== null && value !== undefined
}
