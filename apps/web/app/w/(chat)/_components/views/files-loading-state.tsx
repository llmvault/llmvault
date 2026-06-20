import { Spinner } from "@heroui/react"
import { TreeSkeleton } from "./files-tree-skeleton"

export function FilesLoadingState({ label }: { label: string }) {
  return (
    <div className="flex h-full flex-col gap-4 px-4 py-5">
      <div className="flex items-center gap-2 text-sm text-muted">
        <Spinner size="sm" aria-label={label} />
        {label}
      </div>
      <TreeSkeleton />
    </div>
  )
}
