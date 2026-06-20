"use client"

import { Button } from "@heroui/react"
import { Icon } from "@iconify/react"
import { cn } from "@/lib/utils"
import type { OrgDriveUploadItem } from "@/hooks/use-org-drive-file-uploads"
import type { ImageAttachmentMetadata } from "@/app/w/(chat)/_lib/image-attachments"

export type AttachmentStatus = "uploading" | "describing" | "ready" | "error"

export type AttachmentDescriptionState =
  | { status: "describing" }
  | { status: "ready"; metadata: ImageAttachmentMetadata }
  | { status: "error"; error: string }

export type ComposerImageAttachment = {
  upload: OrgDriveUploadItem
  status: AttachmentStatus
  error?: string
  metadata?: ImageAttachmentMetadata
}

export function AttachmentPreviewTray({
  attachments,
  onRetry,
  onRemove,
}: {
  attachments: ComposerImageAttachment[]
  onRetry: (attachment: ComposerImageAttachment) => void
  onRemove: (id: string) => void
}) {
  return (
    <div className="flex flex-wrap gap-2 px-1 pt-0.5">
      {attachments.map((attachment) => (
        <AttachmentPreviewTile
          key={attachment.upload.id}
          attachment={attachment}
          onRetry={() => onRetry(attachment)}
          onRemove={() => onRemove(attachment.upload.id)}
        />
      ))}
    </div>
  )
}

function AttachmentPreviewTile({
  attachment,
  onRetry,
  onRemove,
}: {
  attachment: ComposerImageAttachment
  onRetry: () => void
  onRemove: () => void
}) {
  const fileName =
    attachment.metadata?.filename ||
    attachment.upload.asset?.filename ||
    attachment.upload.file.name ||
    "Image"
  const label =
    attachment.status === "uploading"
      ? "Uploading"
      : attachment.status === "describing"
        ? "Analyzing"
        : attachment.status === "error"
          ? attachment.error || "Image failed"
          : attachment.metadata?.category
            ? humanAttachmentCategory(attachment.metadata.category)
            : "Ready"
  const pending =
    attachment.status === "uploading" || attachment.status === "describing"
  const error = attachment.status === "error"

  return (
    <div
      className={cn(
        "group bg-default/50 relative h-16 w-16 shrink-0 rounded-2xl border shadow-sm sm:h-20 sm:w-20",
        error ? "border-danger/70" : "border-border"
      )}
      title={`${fileName}: ${label}`}
    >
      {/* eslint-disable-next-line @next/next/no-img-element -- upload previews use local object URLs */}
      <img
        src={attachment.upload.previewUrl}
        alt={fileName}
        className={cn(
          "h-full w-full rounded-[7px] object-cover",
          pending && "opacity-60",
          error && "opacity-75"
        )}
      />
      {pending ? (
        <div className="absolute inset-0 flex items-center justify-center rounded-[7px] bg-background/50">
          <Icon
            icon="lucide:loader-2"
            className="h-4 w-4 animate-spin text-muted"
          />
        </div>
      ) : null}
      {attachment.status === "ready" ? (
        <span className="absolute bottom-1.5 left-1.5 flex h-5 w-5 items-center justify-center rounded-full bg-background/90 text-muted shadow-sm">
          <Icon icon="lucide:check" className="h-3.5 w-3.5" />
        </span>
      ) : null}
      {error ? (
        <Button
          variant="ghost"
          size="sm"
          isIconOnly
          aria-label={`Retry ${fileName}`}
          onPress={onRetry}
          className="bg-danger text-danger-foreground absolute bottom-1.5 left-1.5 z-10 h-5 w-5 min-w-5 rounded-full p-0 shadow-sm"
        >
          <Icon icon="lucide:rotate-ccw" className="h-3.5 w-3.5" />
        </Button>
      ) : null}
      <Button
        variant="ghost"
        size="sm"
        isIconOnly
        aria-label={`Remove ${fileName}`}
        onPress={onRemove}
        className="absolute top-1 right-1 z-20 h-5 w-5 min-w-5 rounded-full bg-foreground p-0 text-background shadow-sm hover:bg-foreground/90"
      >
        <Icon icon="lucide:x" className="h-3 w-3" />
      </Button>
    </div>
  )
}

function humanAttachmentCategory(category: string) {
  return category
    .split("_")
    .filter(Boolean)
    .map((part) => part.slice(0, 1).toUpperCase() + part.slice(1))
    .join(" ")
}
