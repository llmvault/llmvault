"use client"

import { Icon } from "@iconify/react"
import { MarkdownProse } from "@/app/w/(chat)/_components/markdown-prose"
import { CanvasDesignCards } from "@/app/w/(chat)/_components/conversation-canvas-card"
import { assetPreviewAttachments } from "@/app/w/(chat)/_lib/asset-preview-links"
import {
  canvasDesignTargets,
  type CanvasDesignTarget,
} from "@/app/w/(chat)/_lib/canvas-design-links"
import type {
  ConversationBlock,
  MediaAttachment,
} from "@/app/w/(chat)/_lib/static-data"

export function AssistantBlock({
  block,
  onOpenAttachment,
  onOpenCanvasTarget,
}: {
  block: Extract<ConversationBlock, { type: "assistant" }>
  onOpenAttachment: (attachment: MediaAttachment) => void
  onOpenCanvasTarget?: (target: CanvasDesignTarget) => void
}) {
  const previews = assistantPreviewAttachments(block)
  const canvasTargets = block.streaming ? [] : canvasDesignTargets(block.text)
  const showFooter = !block.streaming && Boolean(block.completedAt)

  if (!previews.length && !canvasTargets.length) {
    return (
      <div className="group/final-message flex min-w-0 flex-col items-start gap-1">
        <MarkdownProse text={block.text} streaming={block.streaming} />
        {showFooter ? <AssistantMessageFooter block={block} /> : null}
      </div>
    )
  }

  return (
    <div className="group/final-message flex min-w-0 flex-col items-start gap-3">
      <MarkdownProse text={block.text} streaming={block.streaming} />
      <CanvasDesignCards targets={canvasTargets} onOpen={onOpenCanvasTarget} />
      {previews.length ? (
        <AssistantAssetPreviews
          items={previews}
          onOpenAttachment={onOpenAttachment}
        />
      ) : null}
      {showFooter ? <AssistantMessageFooter block={block} /> : null}
    </div>
  )
}

function AssistantMessageFooter({
  block,
}: {
  block: Extract<ConversationBlock, { type: "assistant" }>
}) {
  const time = formatAssistantMessageTime(block.completedAt)

  return (
    <div className="flex h-6 items-center gap-1.5 text-muted">
      <AssistantFooterButton label="Good response" icon="lucide:thumbs-up" />
      <AssistantFooterButton label="Bad response" icon="lucide:thumbs-down" />
      <AssistantFooterButton label="Fork chat" icon="lucide:git-fork" />
      {time ? (
        <time
          dateTime={block.completedAt}
          suppressHydrationWarning
          className="text-sm font-medium opacity-0 transition-opacity duration-150 group-focus-within/final-message:opacity-100 group-hover/final-message:opacity-100"
        >
          {time}
        </time>
      ) : null}
    </div>
  )
}

function AssistantFooterButton({
  label,
  icon,
}: {
  label: string
  icon: string
}) {
  return (
    <button
      type="button"
      aria-label={label}
      className="focus-visible:ring-ring flex h-6 w-6 items-center justify-center rounded-md text-muted transition-colors hover:bg-default hover:text-foreground focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none"
    >
      <Icon icon={icon} className="h-3.5 w-3.5" />
    </button>
  )
}

function formatAssistantMessageTime(value?: string) {
  if (!value) return ""
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ""
  return new Intl.DateTimeFormat(undefined, {
    hour: "numeric",
    minute: "2-digit",
  }).format(date)
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
          className="group focus-visible:ring-ring max-w-full overflow-hidden rounded-2xl border border-border bg-surface text-left shadow-sm transition-shadow hover:shadow-md focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none"
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

export function assistantPreviewAttachments(
  block: Extract<ConversationBlock, { type: "assistant" }>
) {
  return assetPreviewAttachments(block.text, block.key ?? block.text)
}

function assistantAssetPreviewImageClass(count: number) {
  if (count <= 1) return "max-h-36 max-w-64"
  if (count === 2) return "max-h-32 max-w-56"
  return "max-h-28 max-w-44"
}
