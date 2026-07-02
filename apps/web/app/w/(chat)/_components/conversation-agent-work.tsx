"use client"

import { useState, type ReactNode } from "react"
import { AppIcon } from "@/components/icon"
import { Collapse } from "@/app/w/(chat)/_components/conversation-collapse"
import type { ConversationBlock } from "@/app/w/(chat)/_lib/static-data"

export function AgentWorkBlock({
  block,
  renderBlock,
}: {
  block: Extract<ConversationBlock, { type: "agent_work" }>
  renderBlock: (block: ConversationBlock, index: number) => ReactNode
}) {
  const [expanded, setExpanded] = useState(block.defaultExpanded ?? false)

  if (block.active) {
    return (
      <div className="flex min-w-0 flex-col gap-3">
        <span className="hivy-shimmer self-start text-sm font-medium text-muted">
          Working
        </span>
        <div className="flex min-w-0 flex-col gap-5">
          {block.blocks.map((child, index) => renderBlock(child, index))}
        </div>
      </div>
    )
  }

  const label = block.duration ? `Worked for ${block.duration}` : "Worked"

  return (
    <div className="flex min-w-0 flex-col gap-3">
      <button
        type="button"
        onClick={() => setExpanded((open) => !open)}
        className="focus-visible:outline-warning flex w-full items-center gap-2 border-b border-border pb-3 text-left text-sm font-normal text-muted transition-colors hover:text-foreground focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2"
        aria-expanded={expanded}
      >
        <span>{label}</span>
        <AppIcon
          icon="chevron-right"
          className={`h-5 w-5 shrink-0 transition-transform ${expanded ? "rotate-90" : ""}`}
        />
      </button>
      <Collapse open={expanded}>
        <div className="flex min-w-0 flex-col gap-5 pb-1">
          {block.blocks.map((child, index) => renderBlock(child, index))}
        </div>
      </Collapse>
    </div>
  )
}
