import { Icon } from "@iconify/react"
import { useEffect, useRef, useState } from "react"
import { PersonAvatar } from "@/app/w/(chat)/_components/person-avatar"
import type { ConversationBlock } from "@/app/w/(chat)/_lib/static-data"

export function WorkingBlock({
  block,
}: {
  block: Extract<ConversationBlock, { type: "working" }>
}) {
  const [elapsed, setElapsed] = useState(0)
  const startRef = useRef<number | null>(null)

  useEffect(() => {
    if (block.duration) {
      return
    }
    const tick = () => {
      if (startRef.current === null) {
        startRef.current = performance.now()
      }
      setElapsed(Math.floor((performance.now() - startRef.current) / 1000))
    }
    tick()
    const id = window.setInterval(tick, 1000)
    return () => window.clearInterval(id)
  }, [block.duration])

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-2 text-sm text-muted">
        {block.by ? (
          <>
            <PersonAvatar person={block.by} size="xs" />
            <span>
              Running {block.by.you ? "your" : `${block.by.name}'s`} request ·
              Working for {block.duration ?? `${elapsed}s`}
            </span>
          </>
        ) : (
          <span>Working for {block.duration ?? `${elapsed}s`}</span>
        )}
      </div>
      <div className="h-px w-full bg-border" />
    </div>
  )
}

export function QueuedBlock({
  block,
}: {
  block: Extract<ConversationBlock, { type: "queued" }>
}) {
  return (
    <div className="flex max-w-[85%] flex-col items-end gap-1 self-end opacity-70">
      <div className="flex items-center gap-1.5 pr-1">
        <PersonAvatar person={block.author} size="xs" />
        <span className="text-xs text-muted">
          {block.author.you ? "You" : block.author.name}
        </span>
        <span className="bg-default flex items-center gap-1 rounded-full px-1.5 py-0.5 text-[11px] text-muted">
          <Icon icon="lucide:clock" className="h-3 w-3" />
          Queued
        </span>
      </div>
      <div className="rounded-2xl border border-dashed border-border px-3.5 py-2.5 text-sm">
        {block.text}
      </div>
    </div>
  )
}
