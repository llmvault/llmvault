"use client"

import { useEffect, useMemo, useRef, useState } from "react"
import { AnimatePresence, motion } from "motion/react"
import { Button } from "@heroui/react"
import { Icon } from "@iconify/react"
import { Lightbox } from "@/app/w/(chat)/_components/lightbox"
import { MarkdownProse } from "@/app/w/(chat)/_components/markdown-prose"
import { useWorkspace } from "@/app/w/(chat)/_components/shell"
import { ToolBlock } from "@/app/w/(chat)/_components/tool-block"
import { assetPreviewAttachments } from "@/app/w/(chat)/_lib/asset-preview-links"
import {
  type ConversationBlock,
  type MediaAttachment,
} from "@/app/w/(chat)/_lib/static-data"

export function Conversation({
  blocks,
  onRetryMessage,
}: {
  blocks: ConversationBlock[]
  onRetryMessage?: (eventID: string, text: string) => void
}) {
  // All attachments across the conversation form one gallery so the
  // lightbox can navigate between every shared image and video.
  const gallery = useMemo(() => blocks.flatMap(blockAttachments), [blocks])
  const [lightboxIndex, setLightboxIndex] = useState<number | null>(null)

  const openAttachment = (attachment: MediaAttachment) => {
    const index = gallery.findIndex((item) => item.id === attachment.id)
    if (index >= 0) {
      setLightboxIndex(index)
    }
  }

  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-5 px-4 pt-6 pb-20">
      {blocks.map((block, index) => (
        <Block
          key={conversationBlockKey(block, index)}
          block={block}
          onOpenAttachment={openAttachment}
          onRetryMessage={onRetryMessage}
        />
      ))}
      <Lightbox
        items={gallery}
        index={lightboxIndex}
        onIndexChange={setLightboxIndex}
        onClose={() => setLightboxIndex(null)}
      />
    </div>
  )
}

function conversationBlockKey(block: ConversationBlock, index: number) {
  if (block.type === "worklog") {
    return `${block.key ?? `${block.type}:${index}`}:${
      block.active ? "active" : "complete"
    }`
  }
  return block.key ?? `${block.type}:${index}`
}

function AttachmentThumbs({
  items,
  onOpen,
}: {
  items: MediaAttachment[]
  onOpen: (attachment: MediaAttachment) => void
}) {
  return (
    <div className="flex flex-wrap gap-2">
      {items.map((item) => (
        <button
          key={item.id}
          type="button"
          onClick={() => onOpen(item)}
          aria-label={`Open ${item.filename}`}
          className="group bg-surface relative overflow-hidden rounded-xl border border-border transition-shadow hover:shadow-md"
        >
          {/* eslint-disable-next-line @next/next/no-img-element -- attachment previews keep their original dimensions */}
          <img
            src={item.kind === "video" ? (item.poster ?? item.url) : item.url}
            alt={item.filename}
            className="h-36 w-auto max-w-60 object-cover transition-transform duration-200 group-hover:scale-[1.02]"
          />
          {item.kind === "video" ? (
            <>
              <span className="absolute inset-0 flex items-center justify-center">
                <span className="flex h-10 w-10 items-center justify-center rounded-full bg-background/80 shadow-sm backdrop-blur-sm">
                  <Icon icon="lucide:play" className="h-4 w-4 translate-x-px" />
                </span>
              </span>
              {item.duration ? (
                <span className="absolute right-1.5 bottom-1.5 rounded-md bg-background/80 px-1.5 py-0.5 text-xs tabular-nums backdrop-blur-sm">
                  {item.duration}
                </span>
              ) : null}
            </>
          ) : null}
        </button>
      ))}
    </div>
  )
}

function Block({
  block,
  onOpenAttachment,
  onRetryMessage,
}: {
  block: ConversationBlock
  onOpenAttachment: (attachment: MediaAttachment) => void
  onRetryMessage?: (eventID: string, text: string) => void
}) {
  switch (block.type) {
    case "assistant":
      return (
        <AssistantBlock block={block} onOpenAttachment={onOpenAttachment} />
      )
    case "activity":
      return <ActivityBlock block={block} />
    case "user":
      return (
        <div className="flex max-w-[85%] flex-col items-end gap-1 self-end">
          {block.author && !block.author.you ? (
            <div className="flex items-center gap-1.5 pr-1">
              <PersonAvatar person={block.author} size="xs" />
              <span className="text-xs text-muted">{block.author.name}</span>
            </div>
          ) : null}
          <div className="bg-default flex flex-col gap-2 rounded-2xl px-3.5 py-2.5 text-sm">
            {block.attachments?.length ? (
              <AttachmentThumbs
                items={block.attachments}
                onOpen={onOpenAttachment}
              />
            ) : null}
            <span>{block.text}</span>
            {block.link ? (
              <a
                href={block.link}
                className="text-danger flex min-w-0 items-center gap-1.5 text-sm underline-offset-2 hover:underline"
              >
                <Icon icon="mdi:github" className="h-4 w-4 shrink-0" />
                <span className="truncate">{block.link}</span>
              </a>
            ) : null}
            <UserMessageStatus block={block} onRetryMessage={onRetryMessage} />
          </div>
        </div>
      )
    case "attachments":
      return <AttachmentThumbs items={block.items} onOpen={onOpenAttachment} />
    case "system":
      return (
        <div className="flex items-center gap-2 self-center py-1 text-xs text-muted">
          <span className="h-1 w-1 rounded-full bg-muted/50" />
          {block.text}
        </div>
      )
    case "error":
      return (
        <div className="border-danger/20 bg-danger/10 text-danger flex items-start gap-2 rounded-lg border px-3 py-2 text-sm">
          <Icon
            icon="lucide:circle-alert"
            className="mt-0.5 h-4 w-4 shrink-0"
          />
          <span className="min-w-0">{block.text}</span>
        </div>
      )
    case "queued":
      return <QueuedBlock block={block} />
    case "worked":
      return <WorkedBlock block={block} />
    case "worklog":
      return (
        <WorklogBlock
          block={block}
          onOpenAttachment={onOpenAttachment}
          onRetryMessage={onRetryMessage}
        />
      )
    case "working":
      return <WorkingBlock block={block} />
    case "tool":
      return <ToolBlock block={block} />
    case "thinking":
      return <ThinkingBlock block={block} />
    case "edits":
      return <EditsBlock block={block} />
    case "actions":
      return <ActionsBlock />
  }
}

function UserMessageStatus({
  block,
  onRetryMessage,
}: {
  block: Extract<ConversationBlock, { type: "user" }>
  onRetryMessage?: (eventID: string, text: string) => void
}) {
  const failed = block.clientStatus === "failed"
  if (!failed) {
    return null
  }

  const canRetry = failed && block.clientEventID && onRetryMessage

  return (
    <span
      className={`flex items-center gap-1.5 self-end rounded-full px-2 py-0.5 text-[11px] ${
        failed ? "bg-danger/10 text-danger" : "bg-surface text-muted"
      }`}
      title={block.clientError}
    >
      {failed ? <Icon icon="lucide:circle-alert" className="h-3 w-3" /> : null}
      <span>{failed ? "Not sent" : "Sending"}</span>
      {canRetry ? (
        <button
          type="button"
          className="font-medium underline-offset-2 hover:underline"
          onClick={() => onRetryMessage(block.clientEventID!, block.text)}
        >
          Retry
        </button>
      ) : null}
    </span>
  )
}

function blockAttachments(block: ConversationBlock): MediaAttachment[] {
  if (block.type === "assistant") {
    return assistantPreviewAttachments(block)
  }
  if (block.type === "user") {
    return block.attachments ?? []
  }
  if (block.type === "attachments") {
    return block.items
  }
  if (block.type === "worklog") {
    return block.blocks.flatMap(blockAttachments)
  }
  return []
}

function AssistantBlock({
  block,
  onOpenAttachment,
}: {
  block: Extract<ConversationBlock, { type: "assistant" }>
  onOpenAttachment: (attachment: MediaAttachment) => void
}) {
  const previews = assistantPreviewAttachments(block)

  if (!previews.length) {
    return <MarkdownProse text={block.text} streaming={block.streaming} />
  }

  return (
    <div className="flex min-w-0 flex-col gap-3">
      <MarkdownProse text={block.text} streaming={block.streaming} />
      <AssistantAssetPreviews
        items={previews}
        onOpenAttachment={onOpenAttachment}
      />
    </div>
  )
}

function assistantPreviewAttachments(
  block: Extract<ConversationBlock, { type: "assistant" }>
) {
  return assetPreviewAttachments(block.text, block.key ?? block.text)
}

function AssistantAssetPreviews({
  items,
  onOpenAttachment,
}: {
  items: MediaAttachment[]
  onOpenAttachment: (attachment: MediaAttachment) => void
}) {
  const imageClassName = assistantAssetPreviewImageClass(items.length)

  return (
    <div className="flex flex-wrap gap-2">
      {items.map((item) => (
        <button
          key={item.id}
          type="button"
          onClick={() => onOpenAttachment(item)}
          aria-label={`Open ${item.filename}`}
          className="group bg-surface max-w-full overflow-hidden rounded-2xl border border-border text-left shadow-sm transition-shadow hover:shadow-md focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none"
        >
          {/* eslint-disable-next-line @next/next/no-img-element -- asset previews are arbitrary API-served images */}
          <img
            src={item.url}
            alt={item.filename}
            className={`${imageClassName} h-auto w-auto object-contain transition-transform duration-200 group-hover:scale-[1.01]`}
          />
        </button>
      ))}
    </div>
  )
}

function assistantAssetPreviewImageClass(count: number) {
  if (count <= 1) return "max-h-36 max-w-64"
  if (count === 2) return "max-h-32 max-w-56"
  return "max-h-28 max-w-44"
}

function WorkingBlock({
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

function QueuedBlock({
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

function PersonAvatar({
  person,
  size = "sm",
}: {
  person: { initials: string; color: string }
  size?: "xs" | "sm"
}) {
  const dims = size === "xs" ? "h-5 w-5 text-[10px]" : "h-6 w-6 text-xs"
  return (
    <span
      className={`flex ${dims} ring-surface shrink-0 items-center justify-center rounded-full font-semibold text-white ring-2`}
      style={{ backgroundColor: person.color }}
    >
      {person.initials}
    </span>
  )
}

function ThinkingBlock({
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

const COLLAPSE_TRANSITION = {
  duration: 0.25,
  ease: [0.32, 0.72, 0, 1] as const,
}

// Height + fade collapse for toggled tool-call / file-summary content.
function Collapse({
  open,
  children,
}: {
  open: boolean
  children: React.ReactNode
}) {
  return (
    <AnimatePresence initial={false}>
      {open ? (
        <motion.div
          initial={{ height: 0, opacity: 0 }}
          animate={{ height: "auto", opacity: 1 }}
          exit={{ height: 0, opacity: 0 }}
          transition={COLLAPSE_TRANSITION}
          className="overflow-hidden"
        >
          {children}
        </motion.div>
      ) : null}
    </AnimatePresence>
  )
}

function ActivityBlock({
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

function WorkedBlock({
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

function WorklogBlock({
  block,
  onOpenAttachment,
  onRetryMessage,
}: {
  block: Extract<ConversationBlock, { type: "worklog" }>
  onOpenAttachment: (attachment: MediaAttachment) => void
  onRetryMessage?: (eventID: string, text: string) => void
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
          {block.blocks.map((child, index) => (
            <Block
              key={conversationBlockKey(child, index)}
              block={child}
              onOpenAttachment={onOpenAttachment}
              onRetryMessage={onRetryMessage}
            />
          ))}
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

function EditsBlock({
  block,
}: {
  block: Extract<ConversationBlock, { type: "edits" }>
}) {
  const [showingAll, setShowingAll] = useState(false)
  const { openView } = useWorkspace()

  return (
    <div className="rounded-2xl border border-border">
      <div className="flex items-center gap-3 px-4 py-3">
        <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-border">
          <Icon icon="lucide:file-diff" className="h-4 w-4 text-muted" />
        </div>
        <div className="flex min-w-0 flex-1 flex-col">
          <span className="text-sm font-medium">
            Edited {block.count} files
          </span>
          <span className="text-sm">
            <span className="text-success">+{block.adds}</span>{" "}
            <span className="text-danger">-{block.dels}</span>
          </span>
        </div>
        <Button variant="ghost" size="sm" className="gap-1.5">
          Undo
          <Icon icon="lucide:rotate-ccw" className="h-3.5 w-3.5" />
        </Button>
        <Button variant="tertiary" size="sm" onPress={() => openView("review")}>
          Review
        </Button>
      </div>
      <div className="border-t border-border px-4 py-2">
        {block.files.map((file) => (
          <FileRow key={file.path} file={file} />
        ))}
        {block.moreFiles?.length ? (
          <>
            <Collapse open={showingAll}>
              {block.moreFiles.map((file) => (
                <FileRow key={file.path} file={file} />
              ))}
            </Collapse>
            <button
              type="button"
              onClick={() => setShowingAll((showing) => !showing)}
              className="flex items-center gap-1.5 py-1.5 text-sm text-muted transition-colors hover:text-foreground"
            >
              <span>
                {showingAll
                  ? "Show fewer files"
                  : `Show ${block.moreFiles.length} more files`}
              </span>
              <Icon
                icon="lucide:chevron-down"
                className={`h-3.5 w-3.5 transition-transform ${showingAll ? "rotate-180" : ""}`}
              />
            </button>
          </>
        ) : null}
      </div>
    </div>
  )
}

function FileRow({
  file,
}: {
  file: { path: string; adds: number; dels: number }
}) {
  return (
    <div className="flex items-center gap-2 py-1.5 text-sm">
      <span className="min-w-0 flex-1 truncate font-mono text-[13px]">
        {file.path}
      </span>
      <span className="text-success">+{file.adds}</span>
      <span className="text-danger">-{file.dels}</span>
    </div>
  )
}

function ActionsBlock() {
  const [copied, setCopied] = useState(false)
  const [vote, setVote] = useState<"up" | "down" | null>(null)

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(
        "Fixed and pushed to main. Root cause was the CI file-length cap."
      )
    } catch {
      // Clipboard can be unavailable (permissions, insecure context); the
      // checkmark still gives design-exploration feedback.
    }
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }

  return (
    <div className="-mt-2 flex items-center gap-1">
      <ActionIcon
        icon={copied ? "lucide:check" : "lucide:copy"}
        label="Copy"
        active={copied}
        onPress={copy}
      />
      <ActionIcon
        icon="lucide:thumbs-up"
        label="Good response"
        active={vote === "up"}
        onPress={() => setVote((value) => (value === "up" ? null : "up"))}
      />
      <ActionIcon
        icon="lucide:thumbs-down"
        label="Bad response"
        active={vote === "down"}
        onPress={() => setVote((value) => (value === "down" ? null : "down"))}
      />
      <ActionIcon icon="lucide:share" label="Share" onPress={() => {}} />
    </div>
  )
}

function ActionIcon({
  icon,
  label,
  active,
  onPress,
}: {
  icon: string
  label: string
  active?: boolean
  onPress: () => void
}) {
  return (
    <button
      type="button"
      aria-label={label}
      onClick={onPress}
      className={`hover:bg-default rounded-lg p-1.5 transition-colors ${
        active ? "text-foreground" : "text-muted hover:text-foreground"
      }`}
    >
      <Icon icon={icon} className="h-4 w-4" />
    </button>
  )
}
