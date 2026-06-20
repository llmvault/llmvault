export function ChannelSkeletonList() {
  return (
    <div className="flex flex-col gap-0.5">
      {Array.from({ length: 6 }).map((_, index) => (
        <div
          key={index}
          className="flex items-center gap-2.5 rounded-lg px-3 py-1.5"
        >
          <span className="bg-default h-4 w-4 shrink-0 rounded" />
          <span className="bg-default h-3.5 flex-1 rounded" />
          <span className="bg-default h-3.5 w-3.5 rounded" />
        </div>
      ))}
    </div>
  )
}

export function SidebarStatusRow({
  label,
  actionLabel,
  onAction,
}: {
  label: string
  actionLabel?: string
  onAction?: () => void
}) {
  return (
    <div className="flex items-center gap-2 rounded-lg px-3 py-1.5 text-sm text-muted">
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
