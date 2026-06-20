import { MarkdownProse } from "@/app/w/(chat)/_components/markdown-prose"
import { assetPreviewAttachments } from "@/app/w/(chat)/_lib/asset-preview-links"
import type {
  ConversationBlock,
  MediaAttachment,
} from "@/app/w/(chat)/_lib/static-data"

export function AssistantBlock({
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
