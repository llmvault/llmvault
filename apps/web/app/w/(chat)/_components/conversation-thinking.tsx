import { MarkdownProse } from "@/app/w/(chat)/_components/markdown-prose"
import type { ConversationBlock } from "@/app/w/(chat)/_lib/static-data"

export function ThinkingBlock({
  block,
}: {
  block: Extract<ConversationBlock, { type: "thinking" }>
}) {
  if (block.text) {
    return (
      <div
        className={`bg-default max-h-56 min-w-0 overflow-y-auto rounded-lg border border-border px-3 py-2.5 text-sm text-muted ${
          block.active ? "hivy-shimmer" : ""
        }`}
      >
        <MarkdownProse text={block.text} streaming={block.active} muted />
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
