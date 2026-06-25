"use client"

import { useEffect, useMemo, useState } from "react"
import type { ReactNode } from "react"
import { Button, Spinner } from "@heroui/react"
import { Icon } from "@iconify/react"
import {
  isFreshCanvasSessionURL,
  type CanvasDesignTarget,
  type CanvasSessionURLEntry,
} from "@/app/w/(chat)/_lib/canvas-design-links"
import {
  selectSessionWorkspace,
  useSessionWorkspaceStore,
} from "@/app/w/(chat)/_stores/session-workspace-store"
import { extractErrorMessage as errorMessage } from "@/lib/api/error"

export function DesignView({ sessionId = "new-chat" }: { sessionId?: string }) {
  const canvas = useSessionWorkspaceStore(
    (state) => selectSessionWorkspace(state, sessionId).canvas
  )
  const setCanvasSessionURL = useSessionWorkspaceStore(
    (state) => state.setCanvasSessionURL
  )
  const [failedRequest, setFailedRequest] = useState<{
    key: string
    message: string
  } | null>(null)
  const [retryNonce, setRetryNonce] = useState(0)

  const target = useMemo(
    () =>
      canvas.targets.find((entry) => entry.key === canvas.activeTargetKey) ??
      null,
    [canvas.activeTargetKey, canvas.targets]
  )
  const cached = target ? canvas.sessionURLs[target.key] : undefined
  const freshCached = isFreshCanvasSessionURL(cached) ? cached : undefined
  const requestKey = target ? `${target.key}:${retryNonce}` : ""
  const failedCurrentRequest = failedRequest?.key === requestKey

  useEffect(() => {
    if (!target || freshCached) return
    const controller = new AbortController()
    void fetchCanvasSessionURL(target, controller.signal)
      .then((entry) => {
        setCanvasSessionURL(sessionId, target.key, entry)
      })
      .catch((fetchError) => {
        if (controller.signal.aborted) return
        setFailedRequest({
          key: requestKey,
          message: errorMessage(fetchError, "Could not open Canvas file."),
        })
      })
    return () => controller.abort()
  }, [freshCached, requestKey, sessionId, setCanvasSessionURL, target])

  if (!target) {
    return (
      <DesignState
        icon="lucide:pen-tool"
        title="No Canvas file selected"
        message="Open a Canvas card from the conversation to load a design file here."
      />
    )
  }

  if (failedCurrentRequest && failedRequest && !freshCached) {
    return (
      <DesignState
        icon="lucide:circle-alert"
        title="Canvas is not available"
        message={failedRequest.message}
        action={
          <Button
            size="sm"
            variant="secondary"
            onPress={() => setRetryNonce((n) => n + 1)}
          >
            Retry
          </Button>
        }
      />
    )
  }

  if (!freshCached) {
    return (
      <div className="flex h-full flex-col gap-4 px-4 py-5">
        <div className="flex items-center gap-2 text-sm text-muted">
          <Spinner size="sm" aria-label="Opening Canvas file" />
          Opening Canvas file
        </div>
        <div className="h-full min-h-0 animate-pulse rounded-lg bg-default" />
      </div>
    )
  }

  return (
    <div className="h-full min-h-0 bg-white">
      <iframe
        key={target.key}
        src={freshCached.url}
        title={`Canvas file ${target.fileId}`}
        allow="clipboard-read; clipboard-write"
        className="h-full w-full border-0"
      />
    </div>
  )
}

function DesignState({
  icon,
  title,
  message,
  action,
}: {
  icon: string
  title: string
  message: string
  action?: ReactNode
}) {
  return (
    <div className="flex h-full items-center justify-center px-6 text-center">
      <div className="flex max-w-sm flex-col items-center gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-lg border border-border bg-background">
          <Icon icon={icon} className="h-5 w-5 text-muted" />
        </div>
        <div className="text-sm font-medium">{title}</div>
        <p className="text-sm leading-6 text-muted">{message}</p>
        {action}
      </div>
    </div>
  )
}

async function fetchCanvasSessionURL(
  target: CanvasDesignTarget,
  signal: AbortSignal
): Promise<CanvasSessionURLEntry> {
  const response = await fetch("/api/proxy/v1/canvas/session-url", {
    method: "POST",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      file_id: target.fileId,
      ...(target.pageId ? { page_id: target.pageId } : {}),
    }),
    signal,
  })
  const data = await responseJSON(response)
  if (!response.ok) {
    throw new Error(errorResponseMessage(data, "Could not open Canvas file."))
  }
  const url = stringValue(data, "url")
  const expiresIn = numberValue(data, "expires_in")
  if (!url || expiresIn === undefined) {
    throw new Error("Canvas session response was incomplete.")
  }
  const cachedAt = Date.now()
  return {
    url,
    cachedAt,
    expiresAt: cachedAt + expiresIn * 1000,
  }
}

async function responseJSON(response: Response) {
  try {
    return (await response.json()) as unknown
  } catch {
    return null
  }
}

function errorResponseMessage(data: unknown, fallback: string) {
  if (!data || typeof data !== "object" || Array.isArray(data)) return fallback
  const value = (data as Record<string, unknown>).error
  return typeof value === "string" && value.trim() ? value : fallback
}

function stringValue(data: unknown, key: string) {
  if (!data || typeof data !== "object" || Array.isArray(data)) return ""
  const value = (data as Record<string, unknown>)[key]
  return typeof value === "string" ? value : ""
}

function numberValue(data: unknown, key: string) {
  if (!data || typeof data !== "object" || Array.isArray(data)) return undefined
  const value = (data as Record<string, unknown>)[key]
  return typeof value === "number" ? value : undefined
}
