export function SessionHistorySkeleton() {
  return (
    <div
      className="mx-auto flex w-full max-w-3xl flex-col gap-5 px-4 py-6"
      role="status"
      aria-label="Loading session"
    >
      <div className="flex max-w-[85%] flex-col items-end gap-1 self-end">
        <div className="bg-default h-4 w-20 animate-pulse rounded" />
        <div className="bg-default h-14 w-72 max-w-full animate-pulse rounded-2xl" />
      </div>
      <div className="flex flex-col gap-3">
        <div className="bg-default h-4 w-28 animate-pulse rounded" />
        <div className="bg-default h-4 w-full max-w-xl animate-pulse rounded" />
        <div className="bg-default h-4 w-5/6 animate-pulse rounded" />
        <div className="bg-default h-4 w-2/3 animate-pulse rounded" />
      </div>
      <div className="bg-default/70 h-24 w-full max-w-lg animate-pulse rounded-xl" />
    </div>
  )
}
