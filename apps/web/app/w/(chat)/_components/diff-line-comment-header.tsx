import { AppIcon } from "@/components/icon"

export function LineCommentHeader({
  icon,
  lineLabel,
  title,
}: {
  icon: string
  lineLabel: string
  title: string
}) {
  return (
    <div className="flex min-h-11 items-center gap-2 border-b border-border px-3">
      <span className="bg-default text-default-foreground flex h-6 w-6 shrink-0 items-center justify-center rounded-full">
        <AppIcon icon={icon} className="h-3.5 w-3.5" />
      </span>
      <span className="min-w-0 flex-1 truncate text-sm font-semibold">
        {title}
      </span>
      <span className="shrink-0 text-sm text-muted">
        Comment on line {lineLabel}
      </span>
    </div>
  )
}
