import {
  canvasDesignTargetKey,
  type CanvasDesignTarget,
} from "@/app/w/(chat)/_lib/canvas-design-links"
import type { components } from "@/lib/api/schema"

export type CanvasProject =
  components["schemas"]["canvasProjectCatalogProjectResponse"]
export type CanvasProjectFile =
  components["schemas"]["canvasProjectCatalogFileResponse"]

export function canvasCatalogFileTarget(
  file: CanvasProjectFile,
  project?: CanvasProject
): CanvasDesignTarget {
  const fileId = file.file_id ?? ""
  const pageId = file.page_id || undefined
  return {
    key: canvasDesignTargetKey(fileId, pageId),
    fileId,
    pageId,
    sourceUrl: file.workspace_url ?? "",
    fileName: file.name || "Untitled file",
    projectName: project?.name || undefined,
  }
}
