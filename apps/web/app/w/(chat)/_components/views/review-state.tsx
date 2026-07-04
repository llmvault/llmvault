import { Skeleton, Spinner } from "@heroui/react"
import { AppIcon } from "@/components/icon"

export function ReviewLoadingState({ label }: { label: string }) {
  return (
    <div className="flex h-full flex-col gap-4 px-4 py-5">
      <div className="flex items-center gap-2 text-sm text-muted">
        <Spinner size="sm" aria-label={label} />
        {label}
      </div>
      <div className="flex flex-col gap-3">
        {Array.from({ length: 3 }).map((_, index) => (
          <div
            key={index}
            className="overflow-hidden rounded-lg border border-border bg-background"
          >
            <div className="h-9 border-b border-border px-3 py-2">
              <Skeleton className="h-3.5 w-36 rounded" />
            </div>
            <div className="flex flex-col gap-2 p-3">
              {Array.from({ length: 4 }).map((__, lineIndex) => (
                <Skeleton
                  key={lineIndex}
                  className="h-3 rounded"
                  style={{ width: `${60 + ((lineIndex + index) % 4) * 9}%` }}
                />
              ))}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

export function ReviewEmptyState({
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
        <div className="flex h-10 w-10 items-center justify-center rounded-lg border border-border bg-background">
          <AppIcon icon={icon} className="h-5 w-5 text-muted" />
        </div>
        <div className="text-sm font-medium">{title}</div>
        <p className="text-sm leading-6 text-muted">{message}</p>
      </div>
    </div>
  )
}
