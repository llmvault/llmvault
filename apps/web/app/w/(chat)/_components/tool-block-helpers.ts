import type { ToolCallDetail } from "@/app/w/(chat)/_lib/static-data"
import { hivyDiffOptions } from "@/lib/diffs-theme"

export const TOOL_DIFF_OPTIONS = hivyDiffOptions({
  diffStyle: "unified",
})

export function toolBody(detail: ToolCallDetail) {
  const sections: string[] = []
  if (detail.command) sections.push(`$ ${detail.command}`)
  if (!detail.command && detail.url) sections.push(detail.url)
  if (!detail.command && !detail.url && detail.input) {
    sections.push(detail.input)
  }
  if (detail.output) sections.push(detail.output.trimEnd())
  if (detail.error) sections.push(detail.error)
  if (sections.length === 0) return "No output"
  return sections.join("\n")
}

export function toolFailed(detail: ToolCallDetail) {
  return (
    detail.status === "errored" ||
    detail.timedOut === true ||
    Boolean(detail.error) ||
    (typeof detail.exitCode === "number" && detail.exitCode !== 0)
  )
}

export function statusLabel(detail: ToolCallDetail, running?: boolean) {
  if (running) return "Running"
  if (detail.timedOut) return "Timed out"
  if (toolFailed(detail)) return "Failed"
  return "Success"
}

export function toolIcon(detail: ToolCallDetail) {
  return detail.icon || "square-chevron-right"
}

export function hasExpandableDetail(detail: ToolCallDetail) {
  if (detail.category === "web_fetch") {
    return false
  }
  if (detail.category === "web_search") {
    return Boolean(detail.searchResults?.length || detail.error)
  }
  if (detail.category === "file_read") {
    return Boolean(detail.path || detail.paths?.length)
  }
  if (detail.category === "file_edit" || detail.category === "file_write") {
    return true
  }
  return Boolean(detail.command || detail.output || detail.error)
}

export function splitPatches(diff: string) {
  const clean = diff.trim()
  if (!clean) return []
  const patches = clean.includes("\ndiff --git ")
    ? clean.split(/(?=^diff --git )/m)
    : [clean]
  return patches.map((patch) => `${patch.trimEnd()}\n`).filter(Boolean)
}

export function displayFileName(path: string) {
  const normalized = path.replace(/\\/g, "/")
  return normalized.split("/").filter(Boolean).pop() || path
}

export function formatBytes(bytes: number) {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 102.4) / 10} KB`
  return `${Math.round(bytes / 1024 / 102.4) / 10} MB`
}
