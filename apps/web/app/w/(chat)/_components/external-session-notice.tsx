import { Icon } from "@iconify/react"
import type { ExternalSessionContinuation } from "@/app/w/(chat)/_lib/external-session"

export function ExternalSessionNotice({
  continuation,
}: {
  continuation: ExternalSessionContinuation
}) {
  const label = `Continue in ${continuation.providerLabel}`
  return (
    <div className="pointer-events-auto rounded-lg border border-border bg-surface px-4 py-3 shadow-sm">
      <div className="flex min-w-0 items-center gap-3">
        <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-default text-muted">
          <Icon icon="lucide:messages-square" className="h-4 w-4" />
        </div>
        <p className="min-w-0 flex-1 text-sm text-foreground">
          Please continue this conversation in {continuation.providerLabel}.
        </p>
        {continuation.url ? (
          <a
            href={continuation.url}
            target="_blank"
            rel="noreferrer"
            className="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-md border border-border px-2.5 text-sm font-medium text-foreground transition-colors hover:bg-default"
          >
            {label}
            <Icon icon="lucide:external-link" className="h-3.5 w-3.5" />
          </a>
        ) : null}
      </div>
    </div>
  )
}
