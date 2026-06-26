"use client"

import { useLayoutEffect, useRef } from "react"
import { MarkdownProse } from "@/app/w/(chat)/_components/markdown-prose"
import type { ConversationBlock } from "@/app/w/(chat)/_lib/static-data"

export function ThinkingBlock({
  block,
}: {
  block: Extract<ConversationBlock, { type: "thinking" }>
}) {
  const scrollRef = useRef<HTMLDivElement | null>(null)

  useLayoutEffect(() => {
    if (!block.active || !block.text) return
    const frame = window.requestAnimationFrame(() => {
      const node = scrollRef.current
      if (!node) return
      node.scrollTop = node.scrollHeight
    })
    return () => window.cancelAnimationFrame(frame)
  }, [block.active, block.text])

  if (block.text) {
    return (
      <div
        ref={scrollRef}
        className="max-h-56 min-w-0 overflow-y-auto overscroll-contain rounded-lg border border-border bg-default px-3 py-2.5 text-sm text-muted"
      >
        <MarkdownProse text={block.text} muted />
      </div>
    )
  }

  const label = block.label ?? "Thinking"

  if (!block.active) {
    return (
      <span className="self-start text-sm font-medium text-muted">{label}</span>
    )
  }

  return (
    <span className="hivy-shimmer self-start text-sm font-medium">{label}</span>
  )
}
