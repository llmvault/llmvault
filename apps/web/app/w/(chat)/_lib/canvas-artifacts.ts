"use client"

/**
 * Canvas artifacts API — projects, artifacts, sandbox preview URLs, and the
 * artifact-comment session message.
 *
 * Every backend call goes through the generated `$api` hooks (openapi-react-query
 * over openapi-fetch); there is no hand-written fetch or `/api/proxy` URL here.
 * Request/response shapes come from the OpenAPI `paths`, so backend contract
 * drift fails typecheck. The only bespoke layer is the pure normalizers that map
 * the generated response into the camelCase domain shapes the Canvas UI consumes.
 */

import { useMemo } from "react"
import { $api } from "@/lib/api/hooks"

export type CanvasArtifactType = "web_page" | "presentation" | string
export type CanvasViewportMode = "desktop" | "tablet" | "mobile"

export interface CanvasArtifactProject {
  id: string
  slug?: string
  name: string
  description?: string
  artifactCount?: number
  artifacts: CanvasArtifact[]
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

interface CanvasArtifactPreviewURL {
  url: string
  expiresAt?: number
  expiresIn?: number
}

// Declared as a type alias (not an interface) so it carries an implicit index
// signature and is assignable to the generated `JSON` body element type when
// posted as an artifact comment.
type CanvasArtifactCommentPayload = {
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

// ---------------------------------------------------------------------------
// Hooks (thin composition over the generated `$api` hooks)
// ---------------------------------------------------------------------------

/**
 * Canvas projects (with embedded artifacts) for the active session, mapped to
 * the domain shape. `data` is always a normalized array. Pass `sessionId` null
 * to load the org-wide project list.
 */
export function useCanvasProjects(
  sessionId: string | null,
  options?: { enabled?: boolean }
) {
  const query = $api.useQuery(
    "get",
    "/v1/canvas/projects",
    sessionId ? { params: { query: { session_id: sessionId } } } : {},
    { enabled: options?.enabled ?? true, retry: false }
  )
  const projects = useMemo(
    () => normalizeCanvasProjectList(query.data),
    [query.data]
  )
  return { ...query, data: projects }
}

/**
 * Canvas artifacts for the org, optionally filtered by project/session, mapped
 * to the domain shape. `data` is always a normalized array.
 */
function useCanvasArtifacts(
  input: { projectId?: string | null; sessionId?: string | null } = {},
  options?: { enabled?: boolean }
) {
  const query = $api.useQuery(
    "get",
    "/v1/canvas/artifacts",
    {
      params: {
        query: {
          ...(input.projectId ? { project_id: input.projectId } : {}),
          ...(input.sessionId ? { session_id: input.sessionId } : {}),
        },
      },
    },
    { enabled: options?.enabled ?? true, retry: false }
  )
  const artifacts = useMemo(
    () => normalizeCanvasArtifactList(query.data),
    [query.data]
  )
  return { ...query, data: artifacts }
}

/**
 * A single Canvas artifact, mapped to the domain shape. `data` is null until the
 * artifact loads; the query is disabled until an `artifactId` is provided.
 */
export function useCanvasArtifact(
  artifactId: string | null | undefined,
  options?: { enabled?: boolean }
) {
  const query = $api.useQuery(
    "get",
    "/v1/canvas/artifacts/{artifactID}",
    { params: { path: { artifactID: artifactId ?? "" } } },
    { enabled: (options?.enabled ?? true) && Boolean(artifactId), retry: false }
  )
  const artifact = useMemo(
    () => normalizeCanvasArtifact(query.data),
    [query.data]
  )
  return { ...query, data: artifact }
}

/**
 * Mints a short-lived sandbox preview URL for an artifact. Modeled as a query so
 * the URL is fetched-on-mount and cached; disabled until both an artifact and a
 * session are known. `data.url` is the ready-to-embed preview URL.
 */
export function useCanvasArtifactPreviewURL(
  input: { artifactId: string | null | undefined; sessionId: string | null },
  options?: { enabled?: boolean }
) {
  const query = $api.useQuery(
    "post",
    "/v1/canvas/artifacts/{artifactID}/preview-url",
    {
      params: { path: { artifactID: input.artifactId ?? "" } },
      body: { session_id: input.sessionId ?? "" },
    },
    {
      enabled:
        (options?.enabled ?? true) &&
        Boolean(input.artifactId && input.sessionId),
      retry: false,
    }
  )
  const preview = useMemo(
    () => normalizeCanvasPreviewURL(query.data),
    [query.data]
  )
  return { ...query, data: preview }
}

/**
 * Sends an artifact comment as a structured session message. Returns the raw
 * mutation plus a `sendComment` helper that wraps the comment into the message
 * body; it throws when there is no active session.
 */
export function useSendCanvasArtifactComment(sessionId: string | null) {
  const mutation = $api.useMutation("post", "/v1/sessions/{id}/messages")
  const sendComment = (comment: CanvasArtifactCommentPayload) => {
    if (!sessionId) throw new Error("No active session.")
    return mutation.mutateAsync({
      params: { path: { id: sessionId } },
      body: { text: comment.body, artifact_comments: [comment] },
    })
  }
  return { ...mutation, sendComment }
}

// ---------------------------------------------------------------------------
// Pure transforms (domain mappers + payload builder)
// ---------------------------------------------------------------------------

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

function normalizeCanvasProject(
  value: unknown
): CanvasArtifactProject | null {
  const record = recordValue(value)
  if (!record) return null

  const id = stringValue(record.id, record.project_id, record.projectId)
  if (!id) return null
  const artifacts = arrayValue(record.artifacts)
    .map(normalizeCanvasArtifact)
    .filter(isPresent)
    .map((artifact) =>
      artifact.projectId ? artifact : { ...artifact, projectId: id }
    )

  return {
    id,
    slug: stringValue(record.slug),
    name: stringValue(record.name) || "Untitled project",
    description: stringValue(record.description),
    artifactCount: numberValue(
      record.artifact_count,
      record.artifacts_count,
      record.artifactCount,
      artifacts.length
    ),
    artifacts,
    updatedAt: stringValue(record.updated_at, record.updatedAt),
  }
}

function normalizeCanvasArtifact(value: unknown): CanvasArtifact | null {
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
