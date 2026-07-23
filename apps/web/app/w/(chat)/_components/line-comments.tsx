"use client"

import { createContext, useContext, useMemo, type ReactNode } from "react"
import type { AnnotationSide } from "@pierre/diffs/react"
import {
  formatCodeLineCommentLine as formatPersistedCodeLineCommentLine,
  type CodeLineCommentPayload,
} from "@/app/w/(chat)/_lib/code-line-comments"
import {
  selectSessionWorkspace,
  useSessionWorkspaceStore,
} from "@/app/w/(chat)/_stores/session-workspace-store"

export type CodeLineCommentSourceKind = "review" | "file" | "tool"

interface CodeLineCommentSourceInput {
  kind: CodeLineCommentSourceKind
  path: string
  repoId?: string
  repoName?: string
  repoPath?: string
}

export type CodeLineCommentSourceHint = Partial<CodeLineCommentSourceInput> & {
  kind?: CodeLineCommentSourceKind
}

export interface CodeLineCommentSource extends CodeLineCommentSourceInput {
  key: string
  displayPath: string
}

export interface CodeLineComment {
  id: string
  sourceKey: string
  sourceKind: CodeLineCommentSourceKind
  path: string
  displayPath: string
  repoId?: string
  repoName?: string
  repoPath?: string
  lineNumber: number
  side?: AnnotationSide
  body: string
  createdAt: number
}

interface AddCodeLineCommentInput {
  source: CodeLineCommentSource
  lineNumber: number
  side?: AnnotationSide
  body: string
}

const EMPTY_COMMENTS: CodeLineComment[] = []
const LineCommentsContext = createContext<string | null>(null)

export function LineCommentsProvider({
  scopeKey,
  children,
}: {
  scopeKey: string
  children: ReactNode
}) {
  return (
    <LineCommentsContext.Provider value={scopeKey}>
      {children}
    </LineCommentsContext.Provider>
  )
}

export function useCodeLineComments() {
  const scopeKey = useLineCommentsScope()
  return useSessionWorkspaceStore(
    (state) => selectSessionWorkspace(state, scopeKey).lineComments
  )
}

export function useCodeLineCommentsForSource(sourceKey: string) {
  const scopeKey = useLineCommentsScope()
  const comments = useSessionWorkspaceStore(
    (state) => selectSessionWorkspace(state, scopeKey).lineComments
  )
  return useMemo(
    () =>
      comments.length
        ? comments.filter((comment) => comment.sourceKey === sourceKey)
        : EMPTY_COMMENTS,
    [comments, sourceKey]
  )
}

export function useCodeLineCommentActions() {
  const scopeKey = useLineCommentsScope()
  return useMemo(
    () => ({
      addComment(input: AddCodeLineCommentInput) {
        const body = input.body.trim()
        if (!body) return null
        const id = createCommentId()
        useSessionWorkspaceStore.getState().addLineComment(scopeKey, {
          id,
          sourceKey: input.source.key,
          sourceKind: input.source.kind,
          path: input.source.path,
          displayPath: input.source.displayPath,
          repoId: input.source.repoId,
          repoName: input.source.repoName,
          repoPath: input.source.repoPath,
          lineNumber: input.lineNumber,
          side: input.side,
          body,
          createdAt: Date.now(),
        })
        return id
      },
      clearComments() {
        useSessionWorkspaceStore.getState().clearLineComments(scopeKey)
      },
      removeComment(id: string) {
        useSessionWorkspaceStore.getState().removeLineComment(scopeKey, id)
      },
      removeComments(ids: string[]) {
        useSessionWorkspaceStore.getState().removeLineComments(scopeKey, ids)
      },
    }),
    [scopeKey]
  )
}

export function createCodeLineCommentSource(
  input: CodeLineCommentSourceInput
): CodeLineCommentSource {
  const path = normalizePath(input.path) || "Unknown file"
  const repoPath = normalizePath(input.repoPath)
  const displayPath = displayFilePath(path, repoPath)
  const scope =
    input.repoId?.trim() ||
    (repoPath ? `path:${repoPath}` : "") ||
    input.repoName?.trim() ||
    input.kind

  return {
    ...input,
    key: `${scope}:${path}`,
    path,
    repoPath: repoPath || undefined,
    displayPath,
  }
}

export function codeLineCommentPayloads(
  comments: CodeLineComment[]
): CodeLineCommentPayload[] {
  return comments.map((comment) => ({
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
  }))
}

export function formatCodeLineCommentLine(
  comment: Pick<CodeLineComment, "lineNumber" | "side">
) {
  return formatPersistedCodeLineCommentLine(comment)
}

function useLineCommentsScope() {
  const scopeKey = useContext(LineCommentsContext)
  if (!scopeKey) {
    throw new Error("line comments must be used inside LineCommentsProvider")
  }
  return scopeKey
}

function displayFilePath(path: string, repoPath?: string) {
  if (!repoPath) return path
  if (path === repoPath || path.startsWith(`${repoPath}/`)) return path
  return `${repoPath}/${path}`
}

function normalizePath(path?: string) {
  return (
    path
      ?.replace(/\\/g, "/")
      .replace(/^\/+|\/+$/g, "")
      .trim() ?? ""
  )
}

function createCommentId() {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID()
  }
  return `${Date.now()}:${Math.random().toString(36).slice(2)}`
}
