export type CodeLineCommentSide = "additions" | "deletions"

export interface CodeLineCommentPayload {
  id?: string
  source_key?: string
  source_kind?: string
  path?: string
  display_path?: string
  repo_id?: string
  repo_name?: string
  repo_path?: string
  line_number?: number
  side?: CodeLineCommentSide
  body?: string
  created_at?: number | string
}

export interface CodeLineCommentReference {
  id: string
  sourceKey?: string
  sourceKind?: string
  path: string
  displayPath: string
  repoId?: string
  repoName?: string
  repoPath?: string
  lineNumber: number
  side?: CodeLineCommentSide
  body: string
  createdAt?: number | string
}

export function codeLineCommentReferenceFromPayload(
  value: unknown,
  fallbackID: string
): CodeLineCommentReference | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) return null
  const record = value as Record<string, unknown>
  const lineNumber = numberRecordValue(record, "line_number")
  const body = stringRecordValue(record, "body").trim()
  const path = stringRecordValue(record, "path").trim()
  if (!lineNumber || !body || !path) return null

  const side = codeLineCommentSide(record.side)
  const displayPath = stringRecordValue(record, "display_path").trim() || path

  return {
    id: stringRecordValue(record, "id").trim() || fallbackID,
    sourceKey: optionalStringRecordValue(record, "source_key"),
    sourceKind: optionalStringRecordValue(record, "source_kind"),
    path,
    displayPath,
    repoId: optionalStringRecordValue(record, "repo_id"),
    repoName: optionalStringRecordValue(record, "repo_name"),
    repoPath: optionalStringRecordValue(record, "repo_path"),
    lineNumber,
    side,
    body,
    createdAt:
      typeof record.created_at === "number" ||
      typeof record.created_at === "string"
        ? record.created_at
        : undefined,
  }
}

export function codeLineCommentReferenceToPayload(
  comment: CodeLineCommentReference
): CodeLineCommentPayload {
  return {
    id: comment.id,
    source_key: comment.sourceKey,
    source_kind: comment.sourceKind,
    path: comment.path,
    display_path: comment.displayPath,
    repo_id: comment.repoId,
    repo_name: comment.repoName,
    repo_path: comment.repoPath,
    line_number: comment.lineNumber,
    side: comment.side,
    body: comment.body,
    created_at: comment.createdAt,
  }
}

export function formatCodeLineCommentLocation(
  comment: Pick<
    CodeLineCommentReference,
    "displayPath" | "path" | "lineNumber" | "side"
  >
) {
  const path = comment.displayPath || comment.path
  return `${path}:${formatCodeLineCommentLine(comment)}`
}

export function formatCodeLineCommentLine(
  comment: Pick<CodeLineCommentReference, "lineNumber" | "side">
) {
  if (comment.side === "additions") return `R${comment.lineNumber}`
  if (comment.side === "deletions") return `L${comment.lineNumber}`
  return String(comment.lineNumber)
}

function optionalStringRecordValue(
  record: Record<string, unknown>,
  key: string
) {
  const value = stringRecordValue(record, key).trim()
  return value || undefined
}

function stringRecordValue(record: Record<string, unknown>, key: string) {
  const value = record[key]
  return typeof value === "string" ? value : ""
}

function numberRecordValue(record: Record<string, unknown>, key: string) {
  const value = record[key]
  if (typeof value === "number" && Number.isFinite(value)) return value
  if (typeof value !== "string") return 0
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : 0
}

function codeLineCommentSide(value: unknown): CodeLineCommentSide | undefined {
  return value === "additions" || value === "deletions" ? value : undefined
}
