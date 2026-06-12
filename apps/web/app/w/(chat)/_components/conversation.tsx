"use client"

import { Fragment, useEffect, useMemo, useRef, useState } from "react"
import { AnimatePresence, motion } from "motion/react"
import { Button } from "@heroui/react"
import { Icon } from "@iconify/react"
import {
  type ConversationBlock,
  type MediaAttachment,
} from "../_lib/static-data"
import { useWorkspace } from "./shell"
import { Lightbox } from "./lightbox"

export function Conversation({ blocks }: { blocks: ConversationBlock[] }) {
  // All attachments across the conversation form one gallery so the
  // lightbox can navigate between every shared image and video.
  const gallery = useMemo(
    () =>
      blocks.flatMap((block) =>
        block.type === "user"
          ? (block.attachments ?? [])
          : block.type === "attachments"
            ? block.items
            : []
      ),
    [blocks]
  )
  const [lightboxIndex, setLightboxIndex] = useState<number | null>(null)

  const openAttachment = (attachment: MediaAttachment) => {
    const index = gallery.findIndex((item) => item.id === attachment.id)
    if (index >= 0) setLightboxIndex(index)
  }

  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-5 px-4 py-6">
      {blocks.map((block, index) => (
        <Block key={index} block={block} onOpenAttachment={openAttachment} />
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
          className="group relative overflow-hidden rounded-xl border border-border bg-surface transition-shadow hover:shadow-md"
        >
          {/* eslint-disable-next-line @next/next/no-img-element -- static sample thumbnails */}
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

// Splits backtick-marked spans into inline code chips.
export function InlineProse({ text }: { text: string }) {
  const segments = text.split("`")
  return (
    <>
      {segments.map((segment, index) =>
        index % 2 === 1 ? (
          <code
            key={index}
            className="rounded-md bg-default px-1.5 py-0.5 font-mono text-[0.85em]"
          >
            {segment}
          </code>
        ) : (
          <Fragment key={index}>{segment}</Fragment>
        )
      )}
    </>
  )
}

function Block({
  block,
  onOpenAttachment,
}: {
  block: ConversationBlock
  onOpenAttachment: (attachment: MediaAttachment) => void
}) {
  switch (block.type) {
    case "assistant":
      return (
        <p className="text-[15px] leading-7">
          <InlineProse text={block.text} />
        </p>
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
          <div className="flex flex-col gap-2 rounded-2xl bg-default px-4 py-3 text-[15px]">
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
                className="flex min-w-0 items-center gap-1.5 text-sm text-danger underline-offset-2 hover:underline"
              >
                <Icon icon="mdi:github" className="h-4 w-4 shrink-0" />
                <span className="truncate">{block.link}</span>
              </a>
            ) : null}
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
    case "queued":
      return <QueuedBlock block={block} />
    case "worked":
      return <WorkedBlock block={block} />
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

function WorkingBlock({
  block,
}: {
  block: Extract<ConversationBlock, { type: "working" }>
}) {
  const [elapsed, setElapsed] = useState(0)
  const startRef = useRef<number | null>(null)

  useEffect(() => {
    if (block.duration) return
    const tick = () => {
      if (startRef.current === null) startRef.current = performance.now()
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
        <span className="flex items-center gap-1 rounded-full bg-default px-1.5 py-0.5 text-[11px] text-muted">
          <Icon icon="lucide:clock" className="h-3 w-3" />
          Queued
        </span>
      </div>
      <div className="rounded-2xl border border-dashed border-border px-4 py-3 text-[15px]">
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
      className={`flex ${dims} shrink-0 items-center justify-center rounded-full font-semibold text-white ring-2 ring-surface`}
      style={{ backgroundColor: person.color }}
    >
      {person.initials}
    </span>
  )
}

function ToolBlock({
  block,
}: {
  block: Extract<ConversationBlock, { type: "tool" }>
}) {
  return (
    <div className="flex items-center gap-2 text-sm text-muted">
      <Icon
        icon="lucide:square-chevron-right"
        className="h-3.5 w-3.5 shrink-0"
      />
      <span
        className={`min-w-0 flex-1 truncate font-mono text-[13px] ${
          block.running ? "hivy-shimmer" : ""
        }`}
      >
        {block.label}
      </span>
    </div>
  )
}

function ThinkingBlock({
  block,
}: {
  block: Extract<ConversationBlock, { type: "thinking" }>
}) {
  return (
    <motion.span
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      className="hivy-shimmer self-start text-[15px] font-medium"
    >
      {block.label ?? "Thinking"}
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
            <span className="font-mono text-[13px] text-danger underline-offset-2 hover:underline">
              {block.detail.file}
            </span>
            <span className="text-success">+{block.detail.adds}</span>
            <span className="text-danger">-{block.detail.dels}</span>
            <span className="h-1.5 w-1.5 rounded-full bg-danger" />
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
        <div className="flex flex-col gap-1.5 border-l border-border pl-4 pt-2">
          {block.steps.map((step) => (
            <div key={step} className="flex items-start gap-2 text-sm text-muted">
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
          <span className="text-sm font-medium">Edited {block.count} files</span>
          <span className="text-sm">
            <span className="text-success">+{block.adds}</span>{" "}
            <span className="text-danger">-{block.dels}</span>
          </span>
        </div>
        <Button variant="ghost" size="sm" className="gap-1.5 text-muted">
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
      className={`rounded-lg p-1.5 transition-colors hover:bg-default ${
        active ? "text-foreground" : "text-muted hover:text-foreground"
      }`}
    >
      <Icon icon={icon} className="h-4 w-4" />
    </button>
  )
}
