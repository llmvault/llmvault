"use client"

import { Icon } from "@iconify/react"
import {
  formatCodeLineCommentLine,
  type CodeLineComment,
} from "@/app/w/(chat)/_components/line-comments"

export function ComposerLineComments({
  comments,
  onClear,
  onRemove,
}: {
  comments: CodeLineComment[]
  onClear: () => void
  onRemove: (id: string) => void
}) {
  return (
    <div className="group relative flex items-start px-1">
      <button
        type="button"
        aria-label={`${comments.length} ${comments.length === 1 ? "code comment" : "code comments"}`}
        aria-haspopup="dialog"
        className="hover:bg-default bg-surface-secondary focus-visible:ring-offset-surface flex h-8 items-center gap-2 rounded-2xl border border-border px-3 text-sm font-medium transition-colors focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:outline-none"
        onClick={(event) => event.stopPropagation()}
        onPointerDown={(event) => event.stopPropagation()}
      >
        <Icon icon="lucide:message-square" className="h-4 w-4 text-muted" />
        {comments.length} {comments.length === 1 ? "comment" : "comments"}
      </button>
      <div
        className="absolute bottom-full left-1 z-50 hidden w-[min(24rem,calc(100vw-2rem))] pb-2 group-focus-within:block group-hover:block"
        onClick={(event) => event.stopPropagation()}
        onPointerDown={(event) => event.stopPropagation()}
        aria-label="Code comments"
        role="dialog"
      >
        <div className="bg-surface flex max-h-80 w-full flex-col gap-1 overflow-y-auto rounded-2xl border border-border p-2 shadow-xl">
          <div className="flex items-center justify-between px-2 py-1">
            <span className="text-xs font-medium text-muted">
              Comments sent with next message
            </span>
            <button
              type="button"
              className="text-xs text-muted transition-colors hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
              onClick={(event) => {
                event.stopPropagation()
                onClear()
              }}
            >
              Clear
            </button>
          </div>
          {comments.map((comment) => (
            <div
              key={comment.id}
              className="hover:bg-default group/comment rounded-xl px-2 py-2 transition-colors"
            >
              <div className="flex min-w-0 items-start gap-2">
                <div className="min-w-0 flex-1">
                  <div className="truncate font-mono text-xs text-muted">
                    {comment.displayPath}
                  </div>
                  <div className="mt-0.5 text-xs font-medium">
                    Line {formatCodeLineCommentLine(comment)}
                  </div>
                </div>
                <button
                  type="button"
                  aria-label="Remove comment"
                  className="hover:bg-surface-tertiary rounded-md p-1 text-muted opacity-0 transition-opacity group-hover/comment:opacity-100 focus:opacity-100 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
                  onClick={(event) => {
                    event.stopPropagation()
                    onRemove(comment.id)
                  }}
                >
                  <Icon icon="lucide:x" className="h-3.5 w-3.5" />
                </button>
              </div>
              <p className="mt-2 line-clamp-4 text-sm leading-5 whitespace-pre-wrap">
                {comment.body}
              </p>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
