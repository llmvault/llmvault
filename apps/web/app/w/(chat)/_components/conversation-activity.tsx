import { Icon } from "@iconify/react"
import { useState } from "react"
import { Collapse } from "@/app/w/(chat)/_components/conversation-collapse"
import type { ConversationBlock } from "@/app/w/(chat)/_lib/static-data"

export function ActivityBlock({
  block,
}: {
  block: Extract<ConversationBlock, { type: "activity" }>
}) {
  const [expanded, setExpanded] = useState(true)

  return (
    <div className="flex flex-col">
      <button
        type="button"
        onClick={() => setExpanded((open) => !open)}
        className="flex items-center gap-2 text-sm text-muted transition-colors hover:text-foreground"
      >
        <Icon icon="lucide:pencil-line" className="h-3.5 w-3.5" />
        <span>{block.label}</span>
        <Icon
          icon="lucide:chevron-down"
          className={`h-3.5 w-3.5 transition-transform ${expanded ? "" : "-rotate-90"}`}
        />
      </button>
      {block.detail ? (
        <Collapse open={expanded}>
          <div className="flex items-center gap-1.5 pt-1.5 text-sm">
            <span className="text-muted">{block.detail.prefix}</span>
            <span className="text-danger font-mono text-[13px] underline-offset-2 hover:underline">
              {block.detail.file}
            </span>
            <span className="text-success">+{block.detail.adds}</span>
            <span className="text-danger">-{block.detail.dels}</span>
            <span className="bg-danger h-1.5 w-1.5 rounded-full" />
          </div>
        </Collapse>
      ) : null}
    </div>
  )
}

export function WorkedBlock({
  block,
}: {
  block: Extract<ConversationBlock, { type: "worked" }>
}) {
  const [expanded, setExpanded] = useState(false)

  return (
    <div className="flex flex-col">
      <button
        type="button"
        onClick={() => setExpanded((open) => !open)}
        className="flex items-center gap-1.5 self-start text-sm text-muted transition-colors hover:text-foreground"
      >
        <span>Worked for {block.duration}</span>
        <Icon
          icon="lucide:chevron-right"
          className={`h-3.5 w-3.5 transition-transform ${expanded ? "rotate-90" : ""}`}
        />
      </button>
      <Collapse open={expanded}>
        <div className="flex flex-col gap-1.5 border-l border-border pt-2 pl-4">
          {block.steps.map((step) => (
            <div
              key={step}
              className="flex items-start gap-2 text-sm text-muted"
            >
              <Icon
                icon="lucide:check"
                className="mt-0.5 h-3.5 w-3.5 shrink-0 text-success"
              />
              <span>{step}</span>
            </div>
          ))}
        </div>
      </Collapse>
    </div>
  )
}
