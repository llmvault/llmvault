import { AppIcon } from "@/components/icon"
import { AttachmentThumbs } from "@/app/w/(chat)/_components/conversation-attachments"
import { MarkdownProse } from "@/app/w/(chat)/_components/markdown-prose"
import { PersonAvatar } from "@/app/w/(chat)/_components/person-avatar"
import { UserCodeLineComments } from "@/app/w/(chat)/_components/conversation-user-code-line-comments"
import type {
  ConversationBlock,
  MediaAttachment,
} from "@/app/w/(chat)/_lib/static-data"

type AutomatedBadge = { label: string; icon: string }

/**
 * Automated (backend-injected) messages carry a `source` of "trigger",
 * "schedule", or "system". Attribute them to the automation that produced them
 * instead of the viewing user. Returns null for regular user messages.
 */
export function automatedMessageBadge(
  source?: string,
  provider?: string
): AutomatedBadge | null {
  if (source === "schedule") {
    return { label: "Automated · Schedule", icon: "clock" }
  }
  if (source === "system") {
    return { label: "Automated · System", icon: "sparkles" }
  }
  if (source !== "trigger") return null

  const key = (provider ?? "").toLowerCase()
  if (key.startsWith("github")) {
    return { label: "Automated · GitHub", icon: "github" }
  }
  if (key === "slack") return { label: "Automated · Slack", icon: "slack" }
  if (key === "linear") return { label: "Automated · Linear", icon: "linear" }
  if (key === "notion") return { label: "Automated · Notion", icon: "notion" }
  return { label: "Automated", icon: "sparkles" }
}

export function UserMessageBlock({
  block,
  onOpenAttachment,
}: {
  block: Extract<ConversationBlock, { type: "user" }>
  onOpenAttachment: (attachment: MediaAttachment) => void
}) {
  const hasTextBubble = Boolean(block.text || block.link)
  const automated = automatedMessageBadge(block.source, block.provider)

  return (
    <div className="flex max-w-[85%] flex-col items-end gap-1.5 self-end">
      {automated ? (
        <div className="inline-flex items-center gap-1.5 rounded-full border border-border bg-surface px-2 py-0.5 text-xs text-muted">
          <AppIcon icon={automated.icon} className="h-3.5 w-3.5" />
          <span>{automated.label}</span>
        </div>
      ) : block.author && !block.author.you ? (
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
