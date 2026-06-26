import { Button } from "@heroui/react"
import { Icon } from "@iconify/react"
import type { PreviewBrowserTarget } from "@/app/w/(chat)/_lib/preview-browser-links"

export function PreviewBrowserCards({
  targets,
  onOpen,
}: {
  targets: PreviewBrowserTarget[]
  onOpen?: (target: PreviewBrowserTarget) => void
}) {
  if (!targets.length || !onOpen) return null

  return (
    <div className="bg-secondary flex w-full flex-col overflow-hidden rounded-2xl border border-border">
      {targets.map((target, index) => (
        <div
          key={target.key}
          className={`flex min-w-0 items-center gap-3 px-4 py-3 ${
            index > 0 ? "border-t border-border" : ""
          }`}
        >
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-default">
            <Icon icon="lucide:globe" className="h-5 w-5 text-muted" />
          </div>
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm font-medium">Preview app</div>
            <div className="truncate text-sm text-muted">
              {target.port ? `Port ${target.port} / ` : ""}
              {target.host}
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
