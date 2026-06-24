import { Button } from "@heroui/react"
import { Icon } from "@iconify/react"
import type { CanvasDesignTarget } from "@/app/w/(chat)/_lib/canvas-design-links"

export function CanvasDesignCards({
  targets,
  onOpen,
}: {
  targets: CanvasDesignTarget[]
  onOpen?: (target: CanvasDesignTarget) => void
}) {
  if (!targets.length || !onOpen) return null

  return (
    <div className="flex w-full flex-col overflow-hidden rounded-2xl border border-border bg-secondary">
      {targets.map((target, index) => (
        <div
          key={target.key}
          className={`flex min-w-0 items-center gap-3 px-4 py-3 ${
            index > 0 ? "border-t border-border" : ""
          }`}
        >
          <div className="bg-default flex h-10 w-10 shrink-0 items-center justify-center rounded-xl">
            <Icon icon="lucide:pen-tool" className="h-5 w-5 text-muted" />
          </div>
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm font-medium">Canvas file</div>
            <div className="truncate text-sm text-muted">
              Design / {target.fileId}
            </div>
          </div>
          <Button
            size="sm"
            variant="secondary"
            className="shrink-0"
            onPress={() => onOpen(target)}
          >
            Open
          </Button>
        </div>
      ))}
    </div>
  )
}
