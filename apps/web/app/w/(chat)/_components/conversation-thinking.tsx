import { motion } from "motion/react"
import { useState } from "react"
import { Collapse } from "@/app/w/(chat)/_components/conversation-collapse"
import { MarkdownProse } from "@/app/w/(chat)/_components/markdown-prose"
import type { ConversationBlock } from "@/app/w/(chat)/_lib/static-data"

export function ThinkingBlock({
  block,
}: {
  block: Extract<ConversationBlock, { type: "thinking" }>
}) {
  const [expanded, setExpanded] = useState(block.defaultExpanded ?? false)

  if (block.text) {
    return (
      <div className="flex min-w-0 flex-col gap-2">
        <button
          type="button"
          onClick={() => setExpanded((open) => !open)}
          className={`focus-visible:outline-warning self-start text-left text-sm font-medium text-muted transition-colors hover:text-foreground focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 ${
            block.active ? "hivy-shimmer" : ""
          }`}
        >
          {block.label ??
            (block.duration ? `Thought for ${block.duration}` : "Thinking")}
        </button>
        <Collapse open={expanded}>
          <div className="bg-default rounded-2xl px-4 py-3">
            <MarkdownProse text={block.text} streaming={block.active} muted />
          </div>
        </Collapse>
      </div>
    )
  }

  if (block.duration) {
    return (
      <span className="self-start text-sm font-medium text-muted">
        {block.label ?? `Thought for ${block.duration}`}
      </span>
    )
  }

  const label = block.label ?? "Thinking"

  if (!block.active) {
    return (
      <span className="self-start text-sm font-medium text-muted">{label}</span>
    )
  }

  return (
    <motion.span
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      className="hivy-shimmer self-start text-sm font-medium"
    >
      {label}
    </motion.span>
  )
}
