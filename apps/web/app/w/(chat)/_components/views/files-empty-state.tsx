"use client"

import { Button } from "@heroui/react"
import { AppIcon } from "@/components/icon"

export function FilesEmptyState({
  icon,
  title,
  message,
}: {
  icon: string
  title: string
  message: string
}) {
  return (
    <div className="flex h-full items-center justify-center px-6 text-center">
      <div className="flex max-w-sm flex-col items-center gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-xl border border-border bg-background">
          <AppIcon icon={icon} className="h-5 w-5 text-muted" />
        </div>
        <div className="text-sm font-medium">{title}</div>
        <p className="text-sm leading-6 text-muted">{message}</p>
      </div>
    </div>
  )
}

export function FilesErrorState({
  message,
  onRetry,
}: {
  message: string
  onRetry: () => void
}) {
  return (
    <div className="flex h-full items-center justify-center px-6 text-center">
      <div className="flex max-w-sm flex-col items-center gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-xl border border-border bg-background">
          <AppIcon icon="circle-alert" className="h-5 w-5 text-muted" />
        </div>
        <div className="text-sm font-medium">Files are not available</div>
        <p className="text-sm leading-6 text-muted">{message}</p>
        <Button size="sm" variant="secondary" onPress={onRetry}>
          Retry
        </Button>
      </div>
    </div>
  )
}
