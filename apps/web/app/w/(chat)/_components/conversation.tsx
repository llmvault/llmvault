"use client"

import { useMemo, useState } from "react"
import { Icon } from "@iconify/react"
import { Lightbox } from "@/app/w/(chat)/_components/lightbox"
import {
  ToolBlock,
  ToolChainBlock,
} from "@/app/w/(chat)/_components/tool-block"
import { AgentWorkBlock } from "@/app/w/(chat)/_components/conversation-agent-work"
import {
  AssistantBlock,
  assistantPreviewAttachments,
} from "@/app/w/(chat)/_components/conversation-assistant"
import { ThinkingBlock } from "@/app/w/(chat)/_components/conversation-thinking"
import { UserMessageBlock } from "@/app/w/(chat)/_components/conversation-user-message"
import { SubagentBlock } from "@/app/w/(chat)/_components/conversation-subagent"
import type { PreviewBrowserTarget } from "@/app/w/(chat)/_lib/preview-browser-links"
import {
  type ConversationBlock,
  type MediaAttachment,
} from "@/app/w/(chat)/_lib/static-data"

type SubagentConversationBlock = Extract<
  ConversationBlock,
  { type: "subagent" }
>

export function Conversation({
  blocks,
  onOpenPreviewTarget,
  onOpenSubagentRun,
}: {
  blocks: ConversationBlock[]
  onOpenPreviewTarget?: (target: PreviewBrowserTarget) => void
  onOpenSubagentRun?: (block: SubagentConversationBlock) => void
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
          onOpenPreviewTarget={onOpenPreviewTarget}
          onOpenSubagentRun={onOpenSubagentRun}
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
  onOpenPreviewTarget,
  onOpenSubagentRun,
}: {
  block: ConversationBlock
  onOpenAttachment: (attachment: MediaAttachment) => void
  onOpenPreviewTarget?: (target: PreviewBrowserTarget) => void
  onOpenSubagentRun?: (block: SubagentConversationBlock) => void
}) {
  switch (block.type) {
    case "assistant":
      return (
        <AssistantBlock
          block={block}
          onOpenAttachment={onOpenAttachment}
          onOpenPreviewTarget={onOpenPreviewTarget}
        />
      )
    case "user":
      return (
        <UserMessageBlock block={block} onOpenAttachment={onOpenAttachment} />
      )
    case "error":
      return (
        <div className="flex items-start gap-2 rounded-lg border border-danger/20 bg-danger/10 px-3 py-2 text-sm text-danger">
          <Icon
            icon="lucide:circle-alert"
            className="mt-0.5 h-4 w-4 shrink-0"
          />
          <span className="min-w-0">{block.text}</span>
        </div>
      )
    case "agent_work":
      return (
        <AgentWorkBlock
          block={block}
          renderBlock={(child, index) => (
            <Block
              key={conversationBlockKey(child, index)}
              block={child}
              onOpenAttachment={onOpenAttachment}
              onOpenPreviewTarget={onOpenPreviewTarget}
              onOpenSubagentRun={onOpenSubagentRun}
            />
          )}
        />
      )
    case "subagent":
      return <SubagentBlock block={block} onOpenRun={onOpenSubagentRun} />
    case "tool":
      return <ToolBlock block={block} />
    case "tool_chain":
      return <ToolChainBlock block={block} />
    case "thinking":
      return <ThinkingBlock block={block} />
  }
}

function conversationBlockKey(block: ConversationBlock, index: number) {
  if (block.type === "agent_work") {
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
  if (block.type === "agent_work") {
    return block.blocks.flatMap(blockAttachments)
  }
  return []
}
