"use client"

import type { FormEvent, PointerEvent as ReactPointerEvent } from "react"
import { Button } from "@heroui/react"
import { Icon } from "@iconify/react"
import type {
  CommentTarget,
  LineCommentAnnotation,
} from "@/app/w/(chat)/_components/diff-line-comments"
import { LineCommentHeader } from "./diff-line-comment-header"

export function LineCommentAddButton({ onAdd }: { onAdd: () => void }) {
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

export function LineCommentPanel({
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

function formatLineTarget(target: CommentTarget) {
  if (target.kind === "file") {
    return String(target.lineNumber)
  }
  return `${target.side === "additions" ? "R" : "L"}${target.lineNumber}`
}

function stopPointerPropagation(event: ReactPointerEvent<HTMLElement>) {
  event.stopPropagation()
}
