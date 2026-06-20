"use client"

import { useMemo, useState } from "react"
import { Icon } from "@iconify/react"
import { Lightbox } from "@/app/w/(chat)/_components/lightbox"
import { ToolBlock } from "@/app/w/(chat)/_components/tool-block"
import { ActionsBlock } from "@/app/w/(chat)/_components/conversation-actions"
import {
  ActivityBlock,
  WorkedBlock,
} from "@/app/w/(chat)/_components/conversation-activity"
import {
  AssistantBlock,
  assistantPreviewAttachments,
} from "@/app/w/(chat)/_components/conversation-assistant"
import { AttachmentThumbs } from "@/app/w/(chat)/_components/conversation-attachments"
import { EditsBlock } from "@/app/w/(chat)/_components/conversation-edits"
import { ThinkingBlock } from "@/app/w/(chat)/_components/conversation-thinking"
import { UserMessageBlock } from "@/app/w/(chat)/_components/conversation-user-message"
import { WorklogBlock } from "@/app/w/(chat)/_components/conversation-worklog"
import {
  QueuedBlock,
  WorkingBlock,
} from "@/app/w/(chat)/_components/conversation-work-status"
import type { CodeLineCommentReference } from "@/app/w/(chat)/_lib/code-line-comments"
import {
  type ConversationBlock,
  type MediaAttachment,
} from "@/app/w/(chat)/_lib/static-data"

export function Conversation({
  blocks,
  onRetryMessage,
}: {
  blocks: ConversationBlock[]
  onRetryMessage?: (
    eventID: string,
    text: string,
    codeLineComments?: CodeLineCommentReference[]
  ) => void
}) {
  // All attachments across the conversation form one gallery so the
  // lightbox can navigate between every shared image and video.
  const gallery = useMemo(() => blocks.flatMap(blockAttachments), [blocks])
  const [lightboxIndex, setLightboxIndex] = useState<number | null>(null)

  const openAttachment = (attachment: MediaAttachment) => {
    const index = gallery.findIndex((item) => item.id === attachment.id)
    if (index >= 0) {
      setLightboxIndex(index)
    }
  }

  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-5 px-4 pt-6 pb-20">
      {blocks.map((block, index) => (
        <Block
          key={conversationBlockKey(block, index)}
          block={block}
          onOpenAttachment={openAttachment}
          onRetryMessage={onRetryMessage}
        />
      ))}
      <Lightbox
        items={gallery}
        index={lightboxIndex}
        onIndexChange={setLightboxIndex}
        onClose={() => setLightboxIndex(null)}
      />
    </div>
  )
}

function Block({
  block,
  onOpenAttachment,
  onRetryMessage,
}: {
  block: ConversationBlock
  onOpenAttachment: (attachment: MediaAttachment) => void
  onRetryMessage?: (
    eventID: string,
    text: string,
    codeLineComments?: CodeLineCommentReference[]
  ) => void
}) {
  switch (block.type) {
    case "assistant":
      return (
        <AssistantBlock block={block} onOpenAttachment={onOpenAttachment} />
      )
    case "activity":
      return <ActivityBlock block={block} />
    case "user":
      return (
        <UserMessageBlock
          block={block}
          onOpenAttachment={onOpenAttachment}
          onRetryMessage={onRetryMessage}
        />
      )
    case "attachments":
      return <AttachmentThumbs items={block.items} onOpen={onOpenAttachment} />
    case "system":
      return (
        <div className="flex items-center gap-2 self-center py-1 text-xs text-muted">
          <span className="h-1 w-1 rounded-full bg-muted/50" />
          {block.text}
        </div>
      )
    case "error":
      return (
        <div className="border-danger/20 bg-danger/10 text-danger flex items-start gap-2 rounded-lg border px-3 py-2 text-sm">
          <Icon
            icon="lucide:circle-alert"
            className="mt-0.5 h-4 w-4 shrink-0"
          />
          <span className="min-w-0">{block.text}</span>
        </div>
      )
    case "queued":
      return <QueuedBlock block={block} />
    case "worked":
      return <WorkedBlock block={block} />
    case "worklog":
      return (
        <WorklogBlock
          block={block}
          renderBlock={(child, index) => (
            <Block
              key={conversationBlockKey(child, index)}
              block={child}
              onOpenAttachment={onOpenAttachment}
              onRetryMessage={onRetryMessage}
            />
          )}
        />
      )
    case "working":
      return <WorkingBlock block={block} />
    case "tool":
      return <ToolBlock block={block} />
    case "thinking":
      return <ThinkingBlock block={block} />
    case "edits":
      return <EditsBlock block={block} />
    case "actions":
      return <ActionsBlock />
  }
}

function conversationBlockKey(block: ConversationBlock, index: number) {
  if (block.type === "worklog") {
    return `${block.key ?? `${block.type}:${index}`}:${
      block.active ? "active" : "complete"
    }`
  }
  return block.key ?? `${block.type}:${index}`
}

function blockAttachments(block: ConversationBlock): MediaAttachment[] {
  if (block.type === "assistant") {
    return assistantPreviewAttachments(block)
  }
  if (block.type === "user") {
    return block.attachments ?? []
  }
  if (block.type === "attachments") {
    return block.items
  }
  if (block.type === "worklog") {
    return block.blocks.flatMap(blockAttachments)
  }
  return []
}
