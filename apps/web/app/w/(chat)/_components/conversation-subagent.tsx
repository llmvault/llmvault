"use client"

import { Button } from "@heroui/react"
import { Icon } from "@iconify/react"
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
        <Icon
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
        <div className="mt-0.5 truncate text-xs text-muted">
          {block.error || block.preview || block.goal || block.childSessionId}
        </div>
      </div>
      {block.status === "running" ? (
        <Button
          size="sm"
          variant="secondary"
          className="shrink-0"
          onPress={() => onOpenRun?.(block)}
        >
          Open
        </Button>
      ) : null}
    </div>
  )
}

function statusIcon(status: "running" | "completed" | "failed") {
  if (status === "completed") return "lucide:check"
  if (status === "failed") return "lucide:triangle-alert"
  return "lucide:loader-circle"
}
