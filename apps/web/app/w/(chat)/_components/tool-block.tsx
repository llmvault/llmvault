"use client"

import { AppIcon } from "@/components/icon"
import { useState } from "react"
import { BashToolBlock } from "@/app/w/(chat)/_components/bash-tool-block"
import { Collapse } from "@/app/w/(chat)/_components/conversation-collapse"
import {
  hasExpandableDetail,
  toolIcon,
} from "@/app/w/(chat)/_components/tool-block-helpers"
import { ToolDetail } from "@/app/w/(chat)/_components/tool-detail"
import type {
  ConversationBlock,
  ToolCallDetail,
  ToolConversationBlock,
} from "@/app/w/(chat)/_lib/static-data"

export function ToolBlock({
  block,
}: {
  block: Extract<ConversationBlock, { type: "tool" }>
}) {
  const [expanded, setExpanded] = useState(false)
  const expandable = block.detail ? hasExpandableDetail(block.detail) : false

  if (block.detail?.category === "shell") {
    return <BashToolBlock block={block} />
  }

  if (!block.detail) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted">
        <AppIcon
          icon="square-chevron-right"
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
          <AppIcon
            icon="chevron-down"
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

export function ToolChainBlock({
  block,
}: {
  block: Extract<ConversationBlock, { type: "tool_chain" }>
}) {
  const [expanded, setExpanded] = useState(false)
  const labels = block.tools.map(toolChainItemLabel)
  const preview = labels.slice(0, 3).join(", ")
  const overflow = labels.length > 3 ? `, +${labels.length - 3}` : ""

  return (
    <div className="flex min-w-0 flex-col gap-2">
      <button
        type="button"
        onClick={() => setExpanded((open) => !open)}
        className="focus-visible:outline-warning flex w-full min-w-0 items-center gap-2 text-left text-sm text-muted transition-colors hover:text-foreground focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2"
        aria-expanded={expanded}
      >
        <AppIcon icon="workflow" className="h-4 w-4 shrink-0 text-muted" />
        <span className={`shrink-0 ${block.running ? "hivy-shimmer" : ""}`}>
          {block.tools.length} tool calls
        </span>
        {preview ? (
          <span className="min-w-0 flex-1 truncate text-xs text-muted">
            {preview}
            {overflow}
          </span>
        ) : null}
        <AppIcon
          icon="chevron-down"
          className={`h-4 w-4 shrink-0 transition-transform ${expanded ? "rotate-180" : ""}`}
        />
      </button>

      <Collapse open={expanded}>
        <div className="flex max-h-64 flex-col gap-2 overflow-y-auto overscroll-contain border-l border-border pt-1 pl-4">
          {block.tools.map((tool, index) => (
            <ToolBlock
              key={tool.key ?? `${tool.label}:${index}`}
              block={tool}
            />
          ))}
        </div>
      </Collapse>
    </div>
  )
}

function ToolIcon({ detail }: { detail: ToolCallDetail }) {
  const icon = toolIcon(detail)
  return (
    <AppIcon
      icon={icon}
      className={`h-4 w-4 shrink-0 ${
        icon === "chrome" ? "" : "text-muted"
      }`}
    />
  )
}

function toolChainItemLabel(tool: ToolConversationBlock) {
  return tool.label || tool.detail?.kind || tool.detail?.tool
}
