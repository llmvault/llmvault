import {
  canvasDesignTargetKey,
  type CanvasDesignTarget,
} from "@/app/w/(chat)/_lib/canvas-design-links"

export interface CanvasProjectCatalog {
  projects: CanvasProject[]
}

export interface CanvasProject {
  projectId: string
  name: string
  files: CanvasProjectFile[]
}

export interface CanvasProjectFile {
  fileId: string
  projectId: string
  pageId?: string
  name: string
  workspaceURL: string
}

export async function fetchCanvasProjectCatalog(
  signal?: AbortSignal
): Promise<CanvasProjectCatalog> {
  const response = await fetch("/api/proxy/v1/canvas/projects", {
    method: "GET",
    headers: { Accept: "application/json" },
    signal,
  })
  const data = await responseJSON(response)
  if (!response.ok) {
    throw new Error(errorResponseMessage(data, "Could not load Canvas files."))
  }
  return parseCanvasProjectCatalog(data)
}

export function parseCanvasProjectCatalog(data: unknown): CanvasProjectCatalog {
  if (!data || typeof data !== "object" || Array.isArray(data)) {
    return { projects: [] }
  }
  const rawProjects = (data as Record<string, unknown>).projects
  if (!Array.isArray(rawProjects)) return { projects: [] }

  return {
    projects: rawProjects.flatMap((project) => {
      if (!project || typeof project !== "object" || Array.isArray(project)) {
        return []
      }
      const raw = project as Record<string, unknown>
      const projectId = stringValue(raw, "project_id")
      if (!projectId) return []
      const files = raw.files
      return [
        {
          projectId,
          name: stringValue(raw, "name") || "Untitled project",
          files: Array.isArray(files)
            ? files.flatMap((file) => parseCanvasProjectFile(file, projectId))
            : [],
        },
      ]
    }),
  }
}

export function canvasCatalogFileTarget(
  file: CanvasProjectFile,
  project?: CanvasProject
): CanvasDesignTarget {
  return {
    key: canvasDesignTargetKey(file.fileId, file.pageId),
    fileId: file.fileId,
    pageId: file.pageId,
    sourceUrl: file.workspaceURL,
    fileName: file.name,
    projectName: project?.name,
  }
}

function parseCanvasProjectFile(file: unknown, projectId: string) {
  if (!file || typeof file !== "object" || Array.isArray(file)) return []
  const raw = file as Record<string, unknown>
  const fileId = stringValue(raw, "file_id")
  if (!fileId) return []
  return [
    {
      fileId,
      projectId: stringValue(raw, "project_id") || projectId,
      pageId: stringValue(raw, "page_id") || undefined,
      name: stringValue(raw, "name") || "Untitled file",
      workspaceURL: stringValue(raw, "workspace_url"),
    },
  ]
}

async function responseJSON(response: Response) {
  try {
    return (await response.json()) as unknown
  } catch {
    return null
  }
}

function errorResponseMessage(data: unknown, fallback: string) {
  if (!data || typeof data !== "object" || Array.isArray(data)) return fallback
  const value = (data as Record<string, unknown>).error
  return typeof value === "string" && value.trim() ? value : fallback
}

function stringValue(data: Record<string, unknown>, key: string) {
  const value = data[key]
  return typeof value === "string" ? value.trim() : ""
}
