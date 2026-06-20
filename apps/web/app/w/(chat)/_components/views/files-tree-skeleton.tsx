export function TreeSkeleton() {
  return (
    <div className="flex h-full flex-col gap-2 px-3 py-3" role="status">
      {Array.from({ length: 12 }).map((_, index) => (
        <div
          key={index}
          className="flex h-6 items-center gap-2"
          style={{ paddingLeft: `${(index % 4) * 12}px` }}
        >
          <div className="bg-default h-3.5 w-3.5 shrink-0 animate-pulse rounded" />
          <div
            className="bg-default h-3 animate-pulse rounded"
            style={{ width: `${60 + ((index * 17) % 90)}px` }}
          />
        </div>
      ))}
    </div>
  )
}
