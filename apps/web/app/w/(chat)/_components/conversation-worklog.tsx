import { Icon } from "@iconify/react"
import { useEffect, useState, type ReactNode } from "react"
import { Collapse } from "@/app/w/(chat)/_components/conversation-collapse"
import type { ConversationBlock } from "@/app/w/(chat)/_lib/static-data"

export function WorklogBlock({
  block,
  renderBlock,
}: {
  block: Extract<ConversationBlock, { type: "worklog" }>
  renderBlock: (block: ConversationBlock, index: number) => ReactNode
}) {
  const [expanded, setExpanded] = useState(block.defaultExpanded ?? false)
  const duration = useLiveWorkDuration(
    block.active ? block.startedAt : undefined,
    block.duration
  )
  const label = `Worked${duration ? ` for ${duration}` : ""}`

  return (
    <div className="flex min-w-0 flex-col gap-3">
      <button
        type="button"
        onClick={() => setExpanded((open) => !open)}
        className="focus-visible:outline-warning flex w-full items-center gap-2 border-b border-border pb-3 text-left text-sm text-muted transition-colors hover:text-foreground focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2"
      >
        <span className={block.active ? "hivy-shimmer" : ""}>{label}</span>
        <Icon
          icon="lucide:chevron-right"
          className={`h-4 w-4 transition-transform ${
            expanded ? "rotate-90" : ""
          }`}
        />
      </button>
      <Collapse open={expanded}>
        <div className="flex flex-col gap-5 pb-1">
          {block.blocks.map((child, index) => renderBlock(child, index))}
        </div>
      </Collapse>
    </div>
  )
}

function useLiveWorkDuration(startedAt?: number, fallback?: string) {
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    if (!startedAt) {
      return
    }
    const interval = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(interval)
  }, [startedAt])

  if (!startedAt) {
    return fallback
  }
  return formatWorkDuration(Math.max(100, now - startedAt))
}

function formatWorkDuration(durationMs: number) {
  if (durationMs < 1000) {
    return `${Math.max(0.1, Math.round(durationMs / 100) / 10)} seconds`
  }
  if (durationMs < 60_000) {
    const seconds = Math.round(durationMs / 1000)
    return seconds === 1 ? "1 second" : `${seconds} seconds`
  }
  if (durationMs < 3_600_000) {
    const totalSeconds = Math.round(durationMs / 1000)
    const minutes = Math.floor(totalSeconds / 60)
    const seconds = totalSeconds % 60
    return seconds === 0 ? `${minutes}m` : `${minutes}m ${seconds}s`
  }
  const totalMinutes = Math.round(durationMs / 60_000)
  const hours = Math.floor(totalMinutes / 60)
  const minutes = totalMinutes % 60
  return minutes === 0 ? `${hours}h` : `${hours}h ${minutes}m`
}
