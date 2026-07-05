import { Skeleton } from "@heroui/react"

export function SessionHistorySkeleton() {
  return (
    <div
      className="mx-auto flex w-full max-w-3xl flex-col gap-5 px-4 py-6"
      role="status"
      aria-label="Loading session"
    >
      <div className="flex max-w-[85%] flex-col items-end gap-1 self-end">
        <Skeleton className="h-4 w-20 rounded" />
        <Skeleton className="h-14 w-72 max-w-full" />
      </div>
      <div className="flex flex-col gap-3">
        <Skeleton className="h-4 w-28 rounded" />
        <Skeleton className="h-4 w-full max-w-xl rounded" />
        <Skeleton className="h-4 w-5/6 rounded" />
        <Skeleton className="h-4 w-2/3 rounded" />
      </div>
      <Skeleton className="h-24 w-full max-w-lg" />
    </div>
  )
}
