"use client"

import {
  createContext,
  useContext,
  useMemo,
  useSyncExternalStore,
  type ReactNode,
} from "react"
import type { AnnotationSide } from "@pierre/diffs/react"

export type CodeLineCommentSourceKind = "review" | "file" | "tool"

export interface CodeLineCommentSourceInput {
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

interface CodeLineCommentsStore {
  subscribe: (listener: () => void) => () => void
  getSnapshot: () => CodeLineComment[]
  getSourceSnapshot: (sourceKey: string) => CodeLineComment[]
  addComment: (input: AddCodeLineCommentInput) => string | null
  removeComment: (id: string) => void
  removeComments: (ids: string[]) => void
  clearComments: () => void
}

const EMPTY_COMMENTS: CodeLineComment[] = []
const LineCommentsContext = createContext<CodeLineCommentsStore | null>(null)

export function LineCommentsProvider({
  scopeKey,
  children,
}: {
  scopeKey: string
  children: ReactNode
}) {
  const store = useMemo(() => createCodeLineCommentsStore(scopeKey), [scopeKey])
  return (
    <LineCommentsContext.Provider value={store}>
      {children}
    </LineCommentsContext.Provider>
  )
}

export function useCodeLineComments() {
  const store = useLineCommentsStore()
  return useSyncExternalStore(
    store.subscribe,
    store.getSnapshot,
    store.getSnapshot
  )
}

export function useCodeLineCommentsForSource(sourceKey: string) {
  const store = useLineCommentsStore()
  return useSyncExternalStore(
    store.subscribe,
    () => store.getSourceSnapshot(sourceKey),
    () => store.getSourceSnapshot(sourceKey)
  )
}

export function useCodeLineCommentActions() {
  const store = useLineCommentsStore()
  return useMemo(
    () => ({
      addComment: store.addComment,
      clearComments: store.clearComments,
      removeComment: store.removeComment,
      removeComments: store.removeComments,
    }),
    [store]
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

export function composeMessageWithLineComments(
  text: string,
  comments: CodeLineComment[]
) {
  const trimmed = text.trim()
  if (!comments.length) return trimmed
  const commentBlock = [
    "Code comments to address:",
    ...comments.map(
      (comment, index) =>
        `${index + 1}. ${formatCodeLineCommentLocation(comment)}\n${indentCommentBody(comment.body)}`
    ),
  ].join("\n\n")
  return trimmed ? `${trimmed}\n\n${commentBlock}` : commentBlock
}

export function formatCodeLineCommentLocation(comment: CodeLineComment) {
  return `${comment.displayPath}:${formatCodeLineCommentLine(comment)}`
}

export function formatCodeLineCommentLine(
  comment: Pick<CodeLineComment, "lineNumber" | "side">
) {
  if (comment.side === "additions") return `R${comment.lineNumber}`
  if (comment.side === "deletions") return `L${comment.lineNumber}`
  return String(comment.lineNumber)
}

function createCodeLineCommentsStore(scopeKey: string): CodeLineCommentsStore {
  void scopeKey
  let comments = EMPTY_COMMENTS
  let commentsBySource = new Map<string, CodeLineComment[]>()
  const listeners = new Set<() => void>()

  const emit = () => {
    for (const listener of listeners) {
      listener()
    }
  }

  const setComments = (nextComments: CodeLineComment[]) => {
    comments = nextComments.length ? nextComments : EMPTY_COMMENTS
    commentsBySource = comments.reduce((map, comment) => {
      const sourceComments = map.get(comment.sourceKey)
      if (sourceComments) {
        sourceComments.push(comment)
      } else {
        map.set(comment.sourceKey, [comment])
      }
      return map
    }, new Map<string, CodeLineComment[]>())
    emit()
  }

  return {
    subscribe(listener) {
      listeners.add(listener)
      return () => listeners.delete(listener)
    },
    getSnapshot() {
      return comments
    },
    getSourceSnapshot(sourceKey) {
      return commentsBySource.get(sourceKey) ?? EMPTY_COMMENTS
    },
    addComment(input) {
      const body = input.body.trim()
      if (!body) return null
      const id = createCommentId()
      setComments([
        ...comments,
        {
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
        },
      ])
      return id
    },
    removeComment(id) {
      const nextComments = comments.filter((comment) => comment.id !== id)
      if (nextComments.length === comments.length) return
      setComments(nextComments)
    },
    removeComments(ids) {
      if (!ids.length) return
      const removeSet = new Set(ids)
      const nextComments = comments.filter(
        (comment) => !removeSet.has(comment.id)
      )
      if (nextComments.length === comments.length) return
      setComments(nextComments)
    },
    clearComments() {
      if (!comments.length) return
      setComments(EMPTY_COMMENTS)
    },
  }
}

function useLineCommentsStore() {
  const store = useContext(LineCommentsContext)
  if (!store) {
    throw new Error("line comments must be used inside LineCommentsProvider")
  }
  return store
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

function indentCommentBody(body: string) {
  return body
    .trim()
    .split("\n")
    .map((line) => `   ${line}`)
    .join("\n")
}

function createCommentId() {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID()
  }
  return `${Date.now()}:${Math.random().toString(36).slice(2)}`
}
