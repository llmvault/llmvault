"use client"

import { Icon } from "@iconify/react"
import { useState } from "react"
import { Collapse } from "@/app/w/(chat)/_components/conversation-collapse"
import {
  hasExpandableDetail,
  toolIcon,
} from "@/app/w/(chat)/_components/tool-block-helpers"
import { ToolDetail } from "@/app/w/(chat)/_components/tool-detail"
import type {
  ConversationBlock,
  ToolCallDetail,
} from "@/app/w/(chat)/_lib/static-data"

export function ToolBlock({
  block,
}: {
  block: Extract<ConversationBlock, { type: "tool" }>
}) {
  const [expanded, setExpanded] = useState(false)
  const expandable = block.detail ? hasExpandableDetail(block.detail) : false

  if (!block.detail) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted">
        <Icon
          icon="lucide:square-chevron-right"
          className="h-3.5 w-3.5 shrink-0"
        />
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
        <ToolIcon detail={block.detail} />
        <span
          className={`min-w-0 flex-1 truncate text-sm ${
            block.running ? "hivy-shimmer" : ""
          }`}
        >
          {block.label}
        </span>
        {expandable ? (
          <Icon
            icon="lucide:chevron-down"
            className={`h-4 w-4 shrink-0 transition-transform ${expanded ? "rotate-180" : ""}`}
          />
        ) : null}
      </button>

      <Collapse open={expanded}>
        {expandable ? (
          <ToolDetail detail={block.detail} running={block.running} />
        ) : null}
      </Collapse>
    </div>
  )
}

function ToolIcon({ detail }: { detail: ToolCallDetail }) {
  const icon = toolIcon(detail)
  return (
    <Icon
      icon={icon}
      className={`h-4 w-4 shrink-0 ${
        icon === "logos:chrome" ? "" : "text-muted"
      }`}
    />
  )
}
