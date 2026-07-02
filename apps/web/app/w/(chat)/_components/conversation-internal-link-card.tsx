import { AppIcon } from "@/components/icon"
import NextLink from "next/link"
import type { InternalAppLinkTarget } from "@/app/w/(chat)/_lib/internal-app-links"

export function InternalAppLinkCards({
  targets,
}: {
  targets: InternalAppLinkTarget[]
}) {
  if (!targets.length) return null

  return (
    <div className="flex w-full flex-col overflow-hidden rounded-2xl border border-border bg-secondary">
      {targets.map((target, index) => (
        <NextLink
          key={target.key}
          href={target.href}
          className={`group flex min-w-0 items-center gap-3 px-4 py-3 transition-colors hover:bg-default ${
            index > 0 ? "border-t border-border" : ""
          }`}
        >
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-background">
            <AppIcon icon={target.icon} className="h-5 w-5 text-muted" />
          </div>
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm font-medium">{target.label}</div>
            {target.subtitle ? (
              <div className="truncate text-sm text-muted">
                {target.subtitle}
              </div>
            ) : null}
          </div>
          <AppIcon
            icon="chevron-right"
            className="h-4 w-4 shrink-0 text-muted transition-colors group-hover:text-foreground"
            aria-hidden="true"
          />
        </NextLink>
      ))}
    </div>
  )
}
