"use client"

import {
  useCallback,
  useMemo,
  useRef,
  useState,
  type FormEvent,
  type PointerEvent as ReactPointerEvent,
} from "react"
import { Button } from "@heroui/react"
import { Icon } from "@iconify/react"
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

type CommentStatus = "draft" | "saved"

type DiffCommentTarget = {
  kind: "diff"
  lineNumber: number
  side: AnnotationSide
}

type FileCommentTarget = {
  kind: "file"
  lineNumber: number
}

type CommentTarget = DiffCommentTarget | FileCommentTarget

type LineCommentAnnotation = {
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

export function CommentablePatchDiff({
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
}

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

function LineCommentAddButton({ onAdd }: { onAdd: () => void }) {
  return (
    <button
      type="button"
      data-utility-button=""
      aria-label="Add line comment"
      className="hover:bg-default mr-1 flex h-6 w-6 items-center justify-center rounded-md border border-border bg-background text-foreground shadow-sm transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
      onClick={(event) => {
        event.preventDefault()
        event.stopPropagation()
        onAdd()
      }}
      onPointerDown={(event) => {
        event.stopPropagation()
      }}
    >
      <Icon icon="lucide:plus" className="h-4 w-4" />
    </button>
  )
}

function LineCommentPanel({
  annotation,
  onCancel,
  onSubmit,
  onTextChange,
}: {
  annotation: LineCommentAnnotation
  onCancel: (id: string) => void
  onSubmit: (id: string) => void
  onTextChange: (id: string, text: string) => void
}) {
  const lineLabel = formatLineTarget(annotation.target)
  if (annotation.status === "saved") {
    return (
      <div
        className="mx-3 my-2 overflow-hidden rounded-lg border border-border bg-background font-sans text-foreground shadow-lg"
        onPointerDown={stopPointerPropagation}
      >
        <LineCommentHeader
          icon="lucide:message-square"
          lineLabel={lineLabel}
          title="Local comment"
        />
        <div className="px-3 py-3 text-sm leading-6 whitespace-pre-wrap">
          {annotation.text}
        </div>
      </div>
    )
  }

  const canSubmit = annotation.text.trim().length > 0

  return (
    <form
      className="mx-3 my-2 overflow-hidden rounded-lg border border-border bg-background font-sans text-foreground shadow-lg"
      onSubmit={(event: FormEvent<HTMLFormElement>) => {
        event.preventDefault()
        if (canSubmit) {
          onSubmit(annotation.id)
        }
      }}
      onPointerDown={stopPointerPropagation}
    >
      <LineCommentHeader
        icon="lucide:message-square-plus"
        lineLabel={lineLabel}
        title="Local comment"
      />
      <textarea
        autoFocus
        value={annotation.text}
        placeholder="Request change"
        className="min-h-24 w-full resize-y bg-transparent px-3 py-3 text-sm leading-6 text-foreground outline-none placeholder:text-muted"
        onChange={(event) => onTextChange(annotation.id, event.target.value)}
      />
      <div className="flex items-center justify-end gap-2 px-3 pb-3">
        <Button
          size="sm"
          variant="ghost"
          onPress={() => onCancel(annotation.id)}
        >
          Cancel
        </Button>
        <Button
          isDisabled={!canSubmit}
          size="sm"
          type="submit"
          variant="primary"
        >
          Comment
        </Button>
      </div>
    </form>
  )
}

function LineCommentHeader({
  icon,
  lineLabel,
  title,
}: {
  icon: string
  lineLabel: string
  title: string
}) {
  return (
    <div className="flex min-h-11 items-center gap-2 border-b border-border px-3">
      <span className="bg-default text-default-foreground flex h-6 w-6 shrink-0 items-center justify-center rounded-full">
        <Icon icon={icon} className="h-3.5 w-3.5" />
      </span>
      <span className="min-w-0 flex-1 truncate text-sm font-semibold">
        {title}
      </span>
      <span className="shrink-0 text-sm text-muted">
        Comment on line {lineLabel}
      </span>
    </div>
  )
}

function formatLineTarget(target: CommentTarget) {
  if (target.kind === "file") {
    return String(target.lineNumber)
  }
  return `${target.side === "additions" ? "R" : "L"}${target.lineNumber}`
}

function targetKey(target: CommentTarget) {
  if (target.kind === "file") {
    return `file:${target.lineNumber}`
  }
  return `diff:${target.side}:${target.lineNumber}`
}

function stopPointerPropagation(event: ReactPointerEvent<HTMLElement>) {
  event.stopPropagation()
}
