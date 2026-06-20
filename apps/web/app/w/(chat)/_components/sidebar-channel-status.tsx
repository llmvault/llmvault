export function SessionSkeletonList() {
  return (
    <div className="flex flex-col gap-0.5">
      {Array.from({ length: 3 }).map((_, index) => (
        <div
          key={index}
          className="flex items-center gap-2 rounded-lg py-1.5 pr-3 pl-9"
        >
          <span className="bg-default h-3 w-3 shrink-0 rounded-full" />
          <span className="bg-default h-3.5 flex-1 rounded" />
          <span className="bg-default h-3 w-8 rounded" />
        </div>
      ))}
    </div>
  )
}

export function IndentedStatusRow({
  label,
  actionLabel,
  onAction,
  muted = false,
}: {
  label: string
  actionLabel?: string
  onAction?: () => void
  muted?: boolean
}) {
  return (
    <div
      className={`flex items-center gap-2 rounded-lg py-1.5 pr-3 pl-9 text-sm ${
        muted ? "text-muted/60" : "text-muted"
      }`}
    >
      <span className="min-w-0 flex-1 truncate">{label}</span>
      {actionLabel && onAction ? (
        <button
          type="button"
          onClick={onAction}
          className="shrink-0 text-xs transition-colors hover:text-foreground"
        >
          {actionLabel}
        </button>
      ) : null}
    </div>
  )
}
