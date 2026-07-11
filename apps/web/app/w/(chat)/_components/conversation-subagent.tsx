"use client"

import { Button } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import type { ConversationBlock } from "@/app/w/(chat)/_lib/static-data"

type SubagentConversationBlock = Extract<
  ConversationBlock,
  { type: "subagent" }
>

export function SubagentBlock({
  block,
  onOpenRun,
}: {
  block: SubagentConversationBlock
  onOpenRun?: (block: SubagentConversationBlock) => void
}) {
  return (
    <div
      className={`flex w-full min-w-0 items-center gap-3 rounded-lg border px-3 py-2.5 text-left transition-colors ${
        block.selected
          ? "border-border bg-surface"
          : "border-border bg-surface hover:bg-surface-secondary"
      }`}
    >
      <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-default">
        <AppIcon
          icon={statusIcon(block.status)}
          className={`h-4 w-4 text-muted ${
            block.status === "running" ? "animate-spin" : ""
          }`}
        />
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 items-center gap-2">
          <span className="truncate text-sm font-medium text-foreground">
            {block.agentName}
          </span>
        </div>
        {block.jobId || block.childSessionId ? (
          <div
            className="mt-0.5 truncate font-mono text-[11px] text-muted"
            title={block.jobId || block.childSessionId}
          >
            {block.jobId || block.childSessionId}
          </div>
        ) : null}
        <div className="mt-0.5 truncate text-xs text-muted">
          {block.error || block.preview || block.goal || block.childSessionId}
        </div>
      </div>
      <Button
        size="sm"
        variant="secondary"
        className="shrink-0"
        onPress={() => onOpenRun?.(block)}
      >
        {block.status === "running" ? "Open" : "View"}
      </Button>
    </div>
  )
}

function statusIcon(status: "running" | "completed" | "failed") {
  if (status === "completed") return "check"
  if (status === "failed") return "triangle-alert"
  return "loader-circle"
}
