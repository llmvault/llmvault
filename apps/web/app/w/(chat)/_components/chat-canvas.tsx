"use client"

import { useEffect, useRef, useState } from "react"
import { Icon } from "@iconify/react"
import {
  collaborators,
  conversation,
  type ConversationBlock,
} from "../_lib/static-data"
import { AGENTS, agentById, modelById, type Agent } from "../_lib/agents"
import { useWorkspace, type ChatSession } from "./shell"
import { Conversation } from "./conversation"
import { Composer } from "./composer"

const CANNED_REPLY =
  "This is a static design exploration, so I can't actually run that yet — but in the real app I'd pick this up right away. Try the panels on the right: `Review`, `Terminal`, `Browser`, `Files` and `Side chat` are all explorable."

// While the agent is "working", the conversation shows a live status header,
// a couple of reasoning lines, tool-call rows, and a shimmer thinking
// indicator — this tail is appended to the committed blocks during streaming.
const STREAMING_TAIL: ConversationBlock[] = [
  { type: "working", by: collaborators[0] },
  {
    type: "assistant",
    text: "Let me look at what's already set up so I can do this properly instead of just describing it.",
  },
  {
    type: "tool",
    label: 'Ran ls "$CODEX_HOME/automations" 2>/dev/null && rg -n "weekly|status|worked"',
  },
  {
    type: "assistant",
    text: "Nothing matching yet, so I'm checking the local config path before I create anything.",
  },
  { type: "tool", label: "Running printenv CODEX_HOME", running: true },
  { type: "thinking" },
  // A teammate's message arrives mid-run; the agent takes one turn at a time
  // per session, so it waits in the queue until the current request finishes.
  {
    type: "queued",
    author: collaborators[2],
    text: "once that's in, can you also add a Slack notification?",
  },
]

export function ChatCanvas() {
  const { session, startSession } = useWorkspace()

  if (!session) {
    return <NewSessionView onStart={startSession} />
  }

  // Keying by title resets the conversation state whenever a different
  // session is opened from the sidebar or freshly created.
  return <SessionView key={session.title} session={session} />
}

function initialBlocks(session: ChatSession): ConversationBlock[] {
  if (session.initialMessage) {
    return [
      { type: "user", text: session.initialMessage },
      { type: "assistant", text: CANNED_REPLY },
      { type: "actions" },
    ]
  }
  return conversation
}

function SessionView({ session }: { session: ChatSession }) {
  const { setModel } = useWorkspace()
  const agent = agentById(session.agentId)
  const [blocks, setBlocks] = useState<ConversationBlock[]>(() =>
    initialBlocks(session)
  )
  const [streaming, setStreaming] = useState(false)
  const [atBottom, setAtBottom] = useState(true)
  const scrollRef = useRef<HTMLDivElement | null>(null)
  const streamTimer = useRef<number | null>(null)

  useEffect(() => {
    return () => {
      if (streamTimer.current) window.clearTimeout(streamTimer.current)
    }
  }, [])

  const handleScroll = () => {
    const node = scrollRef.current
    if (!node) return
    setAtBottom(node.scrollHeight - node.scrollTop - node.clientHeight < 48)
  }

  const scrollToBottom = (behavior: ScrollBehavior = "smooth") => {
    const node = scrollRef.current
    node?.scrollTo({ top: node.scrollHeight, behavior })
  }

  const finishStream = () => {
    if (streamTimer.current) window.clearTimeout(streamTimer.current)
    streamTimer.current = null
    setStreaming(false)
    setBlocks((current) => [
      ...current,
      { type: "assistant", text: CANNED_REPLY },
      { type: "actions" },
    ])
  }

  const send = (text: string) => {
    if (streaming) return
    setBlocks((current) => [...current, { type: "user", text }])
    setStreaming(true)
    streamTimer.current = window.setTimeout(finishStream, 3200)
  }

  const stop = () => {
    if (streamTimer.current) window.clearTimeout(streamTimer.current)
    streamTimer.current = null
    setStreaming(false)
  }

  // During streaming the working/tool/thinking tail is shown after the
  // committed blocks; it disappears once the reply lands.
  const visibleBlocks = streaming ? [...blocks, ...STREAMING_TAIL] : blocks

  useEffect(() => {
    scrollToBottom("auto")
    // Re-pin to the bottom when messages or the streaming tail change.
  }, [visibleBlocks.length, streaming])

  return (
    <div className="relative flex h-full min-w-0 flex-col">
      <div
        ref={scrollRef}
        onScroll={handleScroll}
        className="min-h-0 flex-1 overflow-y-auto"
      >
        <Conversation blocks={visibleBlocks} />
      </div>

      {!atBottom ? (
        <button
          type="button"
          aria-label="Scroll to bottom"
          onClick={() => scrollToBottom()}
          className="absolute bottom-36 left-1/2 -translate-x-1/2 rounded-full border border-border bg-surface p-2 shadow-sm transition-colors hover:bg-default"
        >
          <Icon icon="lucide:arrow-down" className="h-4 w-4 text-muted" />
        </button>
      ) : null}

      <Composer
        agent={agent}
        modelId={session.modelId}
        onModelChange={setModel}
        onSend={send}
        isStreaming={streaming}
        onStop={stop}
      />
    </div>
  )
}

function NewSessionView({
  onStart,
}: {
  onStart: (agentId: string, firstMessage: string, modelId?: string) => void
}) {
  const [agent, setAgent] = useState<Agent>(AGENTS[0])
  const [modelId, setModelId] = useState(agent.defaultModelId)

  const selectAgent = (next: Agent) => {
    setAgent(next)
    setModelId(next.defaultModelId)
  }

  return (
    <div className="flex h-full min-w-0 flex-col">
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto flex w-full max-w-3xl flex-col gap-6 px-4 py-10">
          <div className="flex flex-col gap-1.5">
            <h1 className="text-xl font-semibold">Start a new session</h1>
            <p className="text-sm text-muted">
              Pick an agent to work with. Each agent runs in its own cloud
              sandbox with its own tools, and stays with the session — you can
              switch models at any time, but not the agent.
            </p>
          </div>

          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            {AGENTS.map((entry) => (
              <AgentCard
                key={entry.id}
                agent={entry}
                selected={entry.id === agent.id}
                onSelect={() => selectAgent(entry)}
              />
            ))}
          </div>
        </div>
      </div>

      <Composer
        agent={agent}
        modelId={modelId}
        onModelChange={setModelId}
        onSend={(text) => onStart(agent.id, text, modelId)}
        placeholder={agent.placeholder}
      />
    </div>
  )
}

function AgentCard({
  agent,
  selected,
  onSelect,
}: {
  agent: Agent
  selected: boolean
  onSelect: () => void
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      aria-pressed={selected}
      className={`flex flex-col gap-2.5 rounded-2xl border p-4 text-left transition-colors ${
        selected
          ? "border-foreground/40 bg-default"
          : "border-border hover:bg-default/60"
      }`}
    >
      <div className="flex items-center gap-2.5">
        <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-border bg-surface">
          <Icon icon={agent.icon} className="h-4 w-4" />
        </span>
        <span className="min-w-0 flex-1 truncate text-sm font-medium">
          {agent.name}
        </span>
        {selected ? (
          <Icon icon="lucide:check" className="h-4 w-4 shrink-0" />
        ) : null}
      </div>
      <p className="text-sm leading-5 text-muted">{agent.description}</p>
      <div className="flex flex-wrap items-center gap-1.5">
        {agent.tools.map((tool) => (
          <span
            key={tool}
            className="rounded-md bg-default px-1.5 py-0.5 text-xs text-muted"
          >
            {tool}
          </span>
        ))}
      </div>
      <div className="flex items-center gap-1.5 text-xs text-muted">
        <span className="flex items-center -space-x-1">
          {agent.modelIds.map((id) => {
            const model = modelById(id)
            return (
              <span
                key={id}
                title={model.label}
                className="flex h-5 w-5 items-center justify-center rounded-full border border-border bg-surface"
              >
                <model.Icon className="h-3 w-3" />
              </span>
            )
          })}
        </span>
        <span>{agent.modelIds.length} models</span>
      </div>
    </button>
  )
}
