import { Icon } from "@iconify/react"
import { AttachmentThumbs } from "@/app/w/(chat)/_components/conversation-attachments"
import { PersonAvatar } from "@/app/w/(chat)/_components/person-avatar"
import { UserCodeLineComments } from "@/app/w/(chat)/_components/conversation-user-code-line-comments"
import type { CodeLineCommentReference } from "@/app/w/(chat)/_lib/code-line-comments"
import type {
  ConversationBlock,
  MediaAttachment,
} from "@/app/w/(chat)/_lib/static-data"

export function UserMessageBlock({
  block,
  onOpenAttachment,
  onRetryMessage,
}: {
  block: Extract<ConversationBlock, { type: "user" }>
  onOpenAttachment: (attachment: MediaAttachment) => void
  onRetryMessage?: (
    eventID: string,
    text: string,
    codeLineComments?: CodeLineCommentReference[]
  ) => void
}) {
  const hasTextBubble = Boolean(block.text || block.link)

  return (
    <div className="flex max-w-[85%] flex-col items-end gap-1.5 self-end">
      {block.author && !block.author.you ? (
        <div className="flex items-center gap-1.5 pr-1">
          <PersonAvatar person={block.author} size="xs" />
          <span className="text-xs text-muted">{block.author.name}</span>
        </div>
      ) : null}
      {block.attachments?.length ? (
        <AttachmentThumbs
          items={block.attachments}
          onOpen={onOpenAttachment}
          align="end"
        />
      ) : null}
      {block.codeLineComments?.length ? (
        <div className="max-w-full">
          <UserCodeLineComments comments={block.codeLineComments} />
        </div>
      ) : null}
      {hasTextBubble ? (
        <div className="bg-default flex max-w-full flex-col gap-2 rounded-2xl px-3.5 py-2.5 text-sm">
          {block.text ? <span>{block.text}</span> : null}
          {block.link ? (
            <a
              href={block.link}
              className="text-danger flex min-w-0 items-center gap-1.5 text-sm underline-offset-2 hover:underline"
            >
              <Icon icon="mdi:github" className="h-4 w-4 shrink-0" />
              <span className="truncate">{block.link}</span>
            </a>
          ) : null}
        </div>
      ) : null}
      <UserMessageStatus block={block} onRetryMessage={onRetryMessage} />
    </div>
  )
}

function UserMessageStatus({
  block,
  onRetryMessage,
}: {
  block: Extract<ConversationBlock, { type: "user" }>
  onRetryMessage?: (
    eventID: string,
    text: string,
    codeLineComments?: CodeLineCommentReference[]
  ) => void
}) {
  const failed = block.clientStatus === "failed"
  if (!failed) {
    return null
  }

  const canRetry = failed && block.clientEventID && onRetryMessage

  return (
    <span
      className={`flex items-center gap-1.5 self-end rounded-full px-2 py-0.5 text-[11px] ${
        failed ? "bg-danger/10 text-danger" : "bg-surface text-muted"
      }`}
      title={block.clientError}
    >
      {failed ? <Icon icon="lucide:circle-alert" className="h-3 w-3" /> : null}
      <span>{failed ? "Not sent" : "Sending"}</span>
      {canRetry ? (
        <button
          type="button"
          className="font-medium underline-offset-2 hover:underline"
          onClick={() =>
            onRetryMessage(
              block.clientEventID!,
              block.text,
              block.codeLineComments
            )
          }
        >
          Retry
        </button>
      ) : null}
    </span>
  )
}
