"use client"

import { Button, Spinner } from "@heroui/react"
import { extractErrorMessage } from "@/lib/api/error"

export function SandboxRuntimeGate({
  sessionId,
  ready,
  pending,
  error,
  onRetry,
}: {
  sessionId?: string
  ready: boolean
  pending: boolean
  error: unknown
  onRetry: () => void
}) {
  if (!sessionId || ready || (!pending && !error)) return null

  return (
    <div className="bg-surface absolute inset-0 z-30 flex items-center justify-center px-4">
      <div
        className="flex max-w-sm flex-col items-center gap-3 text-center"
        role="status"
        aria-live="polite"
      >
        {pending ? <Spinner size="lg" aria-label="Waking sandbox" /> : null}
        <div className="flex flex-col gap-1">
          <p className="text-sm font-medium text-foreground">
            {pending ? "Waking sandbox" : "Sandbox unavailable"}
          </p>
          <p className="text-sm text-muted">
            {pending
              ? "Preparing the agent runtime for this session."
              : extractErrorMessage(error, "Could not wake the sandbox.")}
          </p>
        </div>
        {!pending && error ? (
          <Button variant="secondary" size="sm" onPress={onRetry}>
            Retry
          </Button>
        ) : null}
      </div>
    </div>
  )
}
