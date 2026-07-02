import { AppIcon } from "@/components/icon"
import { AttachmentThumbs } from "@/app/w/(chat)/_components/conversation-attachments"
import { MarkdownProse } from "@/app/w/(chat)/_components/markdown-prose"
import { PersonAvatar } from "@/app/w/(chat)/_components/person-avatar"
import { UserCodeLineComments } from "@/app/w/(chat)/_components/conversation-user-code-line-comments"
import type {
  ConversationBlock,
  MediaAttachment,
} from "@/app/w/(chat)/_lib/static-data"

export function UserMessageBlock({
  block,
  onOpenAttachment,
}: {
  block: Extract<ConversationBlock, { type: "user" }>
  onOpenAttachment: (attachment: MediaAttachment) => void
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
        <div className="flex max-w-full flex-col gap-2 rounded-2xl bg-default px-3.5 py-2.5 text-sm">
          {block.text ? <MarkdownProse text={block.text} /> : null}
          {block.link ? (
            <a
              href={block.link}
              className="flex min-w-0 items-center gap-1.5 text-sm text-danger underline-offset-2 hover:underline"
            >
              <AppIcon icon="github" className="h-4 w-4 shrink-0" />
              <span className="truncate">{block.link}</span>
            </a>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}
