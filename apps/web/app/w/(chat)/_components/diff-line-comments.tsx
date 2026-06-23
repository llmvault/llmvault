"use client"

import { memo, useCallback, useMemo, useRef, useState } from "react"
import { getSingularPatch, type SelectedLineRange } from "@pierre/diffs"
import {
  File as DiffFile,
  PatchDiff,
  type AnnotationSide,
  type DiffLineAnnotation,
  type FileProps,
  type LineAnnotation,
  type PatchDiffProps,
} from "@pierre/diffs/react"
import {
  createCodeLineCommentSource,
  useCodeLineCommentActions,
  useCodeLineCommentsForSource,
  type CodeLineComment,
  type CodeLineCommentSource,
  type CodeLineCommentSourceHint,
  type CodeLineCommentSourceKind,
} from "@/app/w/(chat)/_components/line-comments"
import {
  LineCommentAddButton,
  LineCommentPanel,
} from "./diff-line-comment-controls"

type CommentStatus = "draft" | "saved"

export type DiffCommentTarget = {
  kind: "diff"
  lineNumber: number
  side: AnnotationSide
}

export type FileCommentTarget = {
  kind: "file"
  lineNumber: number
}

export type CommentTarget = DiffCommentTarget | FileCommentTarget

export type LineCommentAnnotation = {
  id: string
  status: CommentStatus
  target: CommentTarget
  text: string
  comment?: CodeLineComment
}

type DiffLineCommentAnnotation = LineCommentAnnotation & {
  target: DiffCommentTarget
}

type FileLineCommentAnnotation = LineCommentAnnotation & {
  target: FileCommentTarget
}

export type CommentablePatchDiffProps = Omit<
  PatchDiffProps<DiffLineCommentAnnotation>,
  | "lineAnnotations"
  | "renderAnnotation"
  | "renderGutterUtility"
  | "selectedLines"
> & {
  source?: CodeLineCommentSourceHint
}

export type CommentableFileProps = Omit<
  FileProps<FileLineCommentAnnotation>,
  | "lineAnnotations"
  | "renderAnnotation"
  | "renderGutterUtility"
  | "selectedLines"
> & {
  source?: CodeLineCommentSourceHint
}

export const CommentablePatchDiff = memo(function CommentablePatchDiff({
  options,
  source,
  ...props
}: CommentablePatchDiffProps) {
  const patchPath = useMemo(
    () => getSingularPatch(props.patch).name,
    [props.patch]
  )
  const resolvedSource = useResolvedCommentSource(source, patchPath, "review")
  const sourceComments = useCodeLineCommentsForSource(resolvedSource.key)
  const draftState = useLineCommentDraft<DiffCommentTarget>(resolvedSource)
  const {
    cancel: cancelDraft,
    draft,
    open: openDraft,
    save: saveDraft,
    updateText: updateDraftText,
  } = draftState
  const optionsWithGutter = useMemo(
    () => ({
      ...options,
      enableGutterUtility: true,
    }),
    [options]
  )
  const selectedLines = useMemo<SelectedLineRange | null>(() => {
    if (!draft) return null
    return {
      start: draft.target.lineNumber,
      end: draft.target.lineNumber,
      side: draft.target.side,
      endSide: draft.target.side,
    }
  }, [draft])
  const lineAnnotations = useMemo<
    DiffLineAnnotation<DiffLineCommentAnnotation>[]
  >(() => {
    const saved = sourceComments.map((comment) => ({
      lineNumber: comment.lineNumber,
      side: comment.side ?? ("additions" as const),
      metadata: savedDiffAnnotation(comment),
    }))
    return draft
      ? [
          ...saved,
          {
            lineNumber: draft.target.lineNumber,
            side: draft.target.side,
            metadata: draft,
          },
        ]
      : saved
  }, [draft, sourceComments])
  const renderGutterUtility = useCallback<
    NonNullable<
      PatchDiffProps<DiffLineCommentAnnotation>["renderGutterUtility"]
    >
  >(
    (getHoveredLine) => (
      <LineCommentAddButton
        onAdd={() => {
          const hoveredLine = getHoveredLine()
          if (!hoveredLine || hoveredLine.lineNumber <= 0) return
          openDraft({
            kind: "diff",
            lineNumber: hoveredLine.lineNumber,
            side: hoveredLine.side,
          })
        }}
      />
    ),
    [openDraft]
  )
  const renderAnnotation = useCallback<
    NonNullable<PatchDiffProps<DiffLineCommentAnnotation>["renderAnnotation"]>
  >(
    (annotation) => (
      <LineCommentPanel
        annotation={annotation.metadata}
        onCancel={cancelDraft}
        onSubmit={saveDraft}
        onTextChange={updateDraftText}
      />
    ),
    [cancelDraft, saveDraft, updateDraftText]
  )

  return (
    <PatchDiff<DiffLineCommentAnnotation>
      {...props}
      options={optionsWithGutter}
      lineAnnotations={lineAnnotations}
      selectedLines={selectedLines}
      renderAnnotation={renderAnnotation}
      renderGutterUtility={renderGutterUtility}
    />
  )
})

export function CommentableFile({
  options,
  source,
  ...props
}: CommentableFileProps) {
  const resolvedSource = useResolvedCommentSource(
    source,
    props.file.name,
    "file"
  )
  const sourceComments = useCodeLineCommentsForSource(resolvedSource.key)
  const draftState = useLineCommentDraft<FileCommentTarget>(resolvedSource)
  const {
    cancel: cancelDraft,
    draft,
    open: openDraft,
    save: saveDraft,
    updateText: updateDraftText,
  } = draftState
  const optionsWithGutter = useMemo(
    () => ({
      ...options,
      enableGutterUtility: true,
    }),
    [options]
  )
  const selectedLines = useMemo<SelectedLineRange | null>(() => {
    if (!draft) return null
    return {
      start: draft.target.lineNumber,
      end: draft.target.lineNumber,
    }
  }, [draft])
  const visibleSourceComments = useMemo(
    () => sourceComments.filter((comment) => comment.side !== "deletions"),
    [sourceComments]
  )
  const lineAnnotations = useMemo<
    LineAnnotation<FileLineCommentAnnotation>[]
  >(() => {
    const saved = visibleSourceComments.map((comment) => ({
      lineNumber: comment.lineNumber,
      metadata: savedFileAnnotation(comment),
    }))
    return draft
      ? [
          ...saved,
          {
            lineNumber: draft.target.lineNumber,
            metadata: draft,
          },
        ]
      : saved
  }, [draft, visibleSourceComments])
  const renderGutterUtility = useCallback<
    NonNullable<FileProps<FileLineCommentAnnotation>["renderGutterUtility"]>
  >(
    (getHoveredLine) => (
      <LineCommentAddButton
        onAdd={() => {
          const hoveredLine = getHoveredLine()
          if (!hoveredLine || hoveredLine.lineNumber <= 0) return
          openDraft({
            kind: "file",
            lineNumber: hoveredLine.lineNumber,
          })
        }}
      />
    ),
    [openDraft]
  )
  const renderAnnotation = useCallback<
    NonNullable<FileProps<FileLineCommentAnnotation>["renderAnnotation"]>
  >(
    (annotation) => (
      <LineCommentPanel
        annotation={annotation.metadata}
        onCancel={cancelDraft}
        onSubmit={saveDraft}
        onTextChange={updateDraftText}
      />
    ),
    [cancelDraft, saveDraft, updateDraftText]
  )

  return (
    <DiffFile<FileLineCommentAnnotation>
      {...props}
      options={optionsWithGutter}
      lineAnnotations={lineAnnotations}
      selectedLines={selectedLines}
      renderAnnotation={renderAnnotation}
      renderGutterUtility={renderGutterUtility}
    />
  )
}

function useResolvedCommentSource(
  source: CodeLineCommentSourceHint | undefined,
  fallbackPath: string,
  fallbackKind: CodeLineCommentSourceKind
) {
  const kind = source?.kind ?? fallbackKind
  const path = source?.path ?? fallbackPath
  const repoId = source?.repoId
  const repoName = source?.repoName
  const repoPath = source?.repoPath

  return useMemo(
    () =>
      createCodeLineCommentSource({
        kind,
        path,
        repoId,
        repoName,
        repoPath,
      }),
    [kind, path, repoId, repoName, repoPath]
  )
}

function useLineCommentDraft<TTarget extends CommentTarget>(
  source: CodeLineCommentSource
) {
  const [draft, setDraft] = useState<
    (LineCommentAnnotation & { target: TTarget; status: "draft" }) | null
  >(null)
  const submittedDraftIdRef = useRef<string | null>(null)
  const commentActions = useCodeLineCommentActions()

  const open = useCallback((target: TTarget) => {
    submittedDraftIdRef.current = null
    setDraft({
      id: `${targetKey(target)}:draft`,
      status: "draft",
      target,
      text: "",
    })
  }, [])

  const cancel = useCallback((id: string) => {
    setDraft((current) => (current?.id === id ? null : current))
  }, [])

  const save = useCallback(
    (id: string) => {
      if (submittedDraftIdRef.current === id) return
      if (!draft || draft.id !== id) return
      const text = draft.text.trim()
      if (!text) {
        setDraft(null)
        return
      }
      submittedDraftIdRef.current = id
      commentActions.addComment({
        source,
        lineNumber: draft.target.lineNumber,
        side: draft.target.kind === "diff" ? draft.target.side : undefined,
        body: text,
      })
      setDraft(null)
    },
    [commentActions, draft, source]
  )

  const updateText = useCallback((id: string, text: string) => {
    setDraft((current) =>
      current?.id === id
        ? {
            ...current,
            text,
          }
        : current
    )
  }, [])

  return useMemo(
    () => ({
      cancel,
      draft,
      open,
      save,
      updateText,
    }),
    [cancel, draft, open, save, updateText]
  )
}

function savedDiffAnnotation(
  comment: CodeLineComment
): DiffLineCommentAnnotation {
  return {
    id: comment.id,
    status: "saved",
    target: {
      kind: "diff",
      lineNumber: comment.lineNumber,
      side: comment.side ?? "additions",
    },
    text: comment.body,
    comment,
  }
}

function savedFileAnnotation(
  comment: CodeLineComment
): FileLineCommentAnnotation {
  return {
    id: comment.id,
    status: "saved",
    target: {
      kind: "file",
      lineNumber: comment.lineNumber,
    },
    text: comment.body,
    comment,
  }
}

function targetKey(target: CommentTarget) {
  if (target.kind === "file") {
    return `file:${target.lineNumber}`
  }
  return `diff:${target.side}:${target.lineNumber}`
}
