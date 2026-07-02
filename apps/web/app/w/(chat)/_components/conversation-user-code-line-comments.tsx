import { AppIcon } from "@/components/icon"
import {
  formatCodeLineCommentLine,
  formatCodeLineCommentLocation,
  type CodeLineCommentReference,
} from "@/app/w/(chat)/_lib/code-line-comments"

export function UserCodeLineComments({
  comments,
}: {
  comments: CodeLineCommentReference[]
}) {
  return (
    <details className="group/comments min-w-0">
      <summary className="hover:bg-surface-secondary bg-surface flex h-7 cursor-pointer list-none items-center gap-2 rounded-full border border-border px-2.5 text-xs font-medium text-foreground transition-colors marker:hidden">
        <AppIcon icon="message-square" className="h-3.5 w-3.5 text-muted" />
        {comments.length} {comments.length === 1 ? "comment" : "comments"}
        <AppIcon
          icon="chevron-down"
          className="ml-auto h-3.5 w-3.5 text-muted transition-transform group-open/comments:rotate-180"
        />
      </summary>
      <div className="mt-2 flex max-w-full flex-col gap-2 border-t border-border/70 pt-2">
        {comments.map((comment) => (
          <div key={comment.id} className="min-w-0">
            <div className="flex min-w-0 items-center gap-2 text-xs">
              <span className="min-w-0 truncate font-mono text-muted">
                {comment.displayPath}
              </span>
              <span className="shrink-0 font-medium">
                {formatCodeLineCommentLine(comment)}
              </span>
            </div>
            <p
              className="mt-1 line-clamp-4 text-sm leading-5 whitespace-pre-wrap"
              title={formatCodeLineCommentLocation(comment)}
            >
              {comment.body}
            </p>
          </div>
        ))}
      </div>
    </details>
  )
}
