import { Skeleton } from "@heroui/react"

export function DetailSkeleton() {
  return (
    <div className="flex flex-col gap-8">
      <header className="flex items-start justify-between gap-4">
        <div className="flex items-center gap-4">
          <Skeleton className="h-12 w-12" />
          <div className="flex flex-col gap-3">
            <Skeleton className="h-5 w-36 rounded" />
            <Skeleton className="h-4 w-80 max-w-full rounded" />
          </div>
        </div>
        <Skeleton className="h-8 w-16 rounded-full" />
      </header>
      <Skeleton className="h-40" />
      <Skeleton className="h-56" />
    </div>
  )
}
