import { Icon } from "@iconify/react"
import { useState } from "react"
import type { MediaAttachment } from "@/app/w/(chat)/_lib/static-data"

export function AttachmentThumbs({
  items,
  onOpen,
  align = "start",
}: {
  items: MediaAttachment[]
  onOpen: (attachment: MediaAttachment) => void
  align?: "start" | "end"
}) {
  const alignment = align === "end" ? "justify-end" : "justify-start"

  return (
    <div className={`flex max-w-full flex-wrap gap-2 ${alignment}`}>
      {items.map((item) => (
        <AttachmentThumb
          key={item.id}
          item={item}
          onOpen={() => onOpen(item)}
        />
      ))}
    </div>
  )
}

function AttachmentThumb({
  item,
  onOpen,
}: {
  item: MediaAttachment
  onOpen: () => void
}) {
  const [loaded, setLoaded] = useState(false)
  const [failed, setFailed] = useState(false)
  const previewSrc =
    item.kind === "video" ? (item.poster ?? item.url) : item.url
  const showSkeleton = !loaded && !failed

  return (
    <button
      type="button"
      onClick={onOpen}
      aria-label={`Open ${item.filename}`}
      className="group bg-surface relative h-36 w-36 shrink-0 overflow-hidden rounded-xl border border-border shadow-sm transition-shadow hover:shadow-md focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background focus-visible:outline-none"
    >
      {showSkeleton ? (
        <span
          aria-hidden="true"
          className="bg-default absolute inset-0 animate-pulse"
        >
          <span className="bg-surface-secondary absolute inset-x-4 bottom-4 h-2 rounded-full" />
        </span>
      ) : null}
      {/* eslint-disable-next-line @next/next/no-img-element -- attachment previews are arbitrary API-served media */}
      <img
        src={previewSrc}
        alt={item.filename}
        onLoad={() => setLoaded(true)}
        onError={() => setFailed(true)}
        className={`h-full w-full object-cover transition-[opacity,transform] duration-200 group-hover:scale-[1.02] ${
          loaded && !failed ? "opacity-100" : "opacity-0"
        }`}
      />
      {failed ? (
        <span className="bg-default absolute inset-0 flex flex-col items-center justify-center gap-2 text-xs text-muted">
          <Icon icon="lucide:image-off" className="h-5 w-5" />
          <span>Preview unavailable</span>
        </span>
      ) : null}
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
  )
}
