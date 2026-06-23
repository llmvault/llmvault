"use client"

import { useState } from "react"
import { Icon } from "@iconify/react"
import { Collapse } from "@/app/w/(chat)/_components/conversation-collapse"
import {
  hasExpandableDetail,
  toolIcon,
} from "@/app/w/(chat)/_components/tool-block-helpers"
import { ToolDetail } from "@/app/w/(chat)/_components/tool-detail"
import type { ConversationBlock } from "@/app/w/(chat)/_lib/static-data"

export function BashToolBlock({
  block,
}: {
  block: Extract<ConversationBlock, { type: "tool" }>
}) {
  const [expanded, setExpanded] = useState(false)
  const expandable = block.detail ? hasExpandableDetail(block.detail) : false
  const actionIcon = block.detail?.actionIcon

  if (!block.detail) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted">
        <Icon icon="lucide:square-terminal" className="h-3.5 w-3.5 shrink-0" />
        <span
          className={`min-w-0 flex-1 truncate font-mono text-sm ${
            block.running ? "hivy-shimmer" : ""
          }`}
        >
          {block.label}
        </span>
      </div>
    )
  }

  return (
    <div className="flex min-w-0 flex-col gap-2">
      <button
        type="button"
        onClick={() => {
          if (expandable) setExpanded((open) => !open)
        }}
        className="focus-visible:outline-warning flex w-full min-w-0 items-center gap-2 text-left text-sm text-muted transition-colors hover:text-foreground focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2"
        aria-expanded={expandable ? expanded : undefined}
      >
        <BashToolIcon block={block} />
        <span
          className={`min-w-0 flex-1 truncate text-sm ${
            block.running ? "hivy-shimmer" : ""
          }`}
        >
          {block.label}
        </span>
        {actionIcon ? (
          <Icon icon={actionIcon} className="h-3.5 w-3.5 shrink-0 text-muted" />
        ) : null}
        {expandable ? (
          <Icon
            icon="lucide:chevron-down"
            className={`h-4 w-4 shrink-0 transition-transform ${expanded ? "rotate-180" : ""}`}
          />
        ) : null}
      </button>

      <Collapse open={expanded}>
        {expandable ? (
          <div className="max-h-64 overflow-y-auto overscroll-contain rounded-lg">
            <ToolDetail detail={block.detail} running={block.running} />
          </div>
        ) : null}
      </Collapse>
    </div>
  )
}

function BashToolIcon({
  block,
}: {
  block: Extract<ConversationBlock, { type: "tool" }>
}) {
  const icon = block.detail ? toolIcon(block.detail) : "lucide:square-terminal"

  return (
    <Icon
      icon={icon}
      className={`h-4 w-4 shrink-0 ${
        icon === "logos:chrome" ? "" : "text-muted"
      }`}
    />
  )
}
