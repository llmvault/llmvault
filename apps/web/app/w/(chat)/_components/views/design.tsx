"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import type { ReactNode } from "react"
import { Button, Spinner } from "@heroui/react"
import { Icon } from "@iconify/react"
import {
  isFreshCanvasSessionURL,
  type CanvasSessionURLEntry,
} from "@/app/w/(chat)/_lib/canvas-design-links"
import {
  selectSessionWorkspace,
  useSessionWorkspaceStore,
} from "@/app/w/(chat)/_stores/session-workspace-store"
import {
  canvasCatalogFileTarget,
  type CanvasProject,
  type CanvasProjectFile,
} from "@/app/w/(chat)/_lib/canvas-projects"
import { extractErrorMessage as errorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"

export function DesignView({ sessionId = "new-chat" }: { sessionId?: string }) {
  const canvas = useSessionWorkspaceStore(
    (state) => selectSessionWorkspace(state, sessionId).canvas
  )
  const openCanvasTarget = useSessionWorkspaceStore(
    (state) => state.openCanvasTarget
  )
  const setCanvasSessionURL = useSessionWorkspaceStore(
    (state) => state.setCanvasSessionURL
  )
  const [showCatalog, setShowCatalog] = useState(false)
  const [failedRequest, setFailedRequest] = useState<{
    key: string
    message: string
  } | null>(null)
  const [retryNonce, setRetryNonce] = useState(0)
  const { mutateAsync: createCanvasSessionURL } = $api.useMutation(
    "post",
    "/v1/canvas/session-url"
  )

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
    if (target) setShowCatalog(false)
  }, [target])

  useEffect(() => {
    if (!target || freshCached) return
    let cancelled = false
    void createCanvasSessionURL({
      body: {
        file_id: target.fileId,
        ...(target.pageId ? { page_id: target.pageId } : {}),
      },
    })
      .then((data) => canvasSessionURLEntryFromResponse(data))
      .then((entry) => {
        if (cancelled) return
        setCanvasSessionURL(sessionId, target.key, entry)
      })
      .catch((fetchError) => {
        if (cancelled) return
        setFailedRequest({
          key: requestKey,
          message: errorMessage(fetchError, "Could not open Canvas file."),
        })
      })
    return () => {
      cancelled = true
    }
  }, [
    createCanvasSessionURL,
    freshCached,
    requestKey,
    sessionId,
    setCanvasSessionURL,
    target,
  ])

  const selectCatalogFile = useCallback(
    (project: CanvasProject, file: CanvasProjectFile) => {
      openCanvasTarget(sessionId, canvasCatalogFileTarget(file, project))
      setFailedRequest(null)
      setRetryNonce((n) => n + 1)
      setShowCatalog(false)
    },
    [openCanvasTarget, sessionId]
  )

  if (!target || showCatalog) {
    return (
      <CanvasProjectPicker
        activeTargetKey={target?.key ?? null}
        canClose={Boolean(target)}
        onClose={() => setShowCatalog(false)}
        onSelectFile={selectCatalogFile}
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
    <div className="flex h-full min-h-0 flex-col bg-white">
      <div className="flex h-11 shrink-0 items-center justify-between border-b border-border bg-surface px-3">
        <div className="flex min-w-0 items-center gap-2">
          <Icon icon="lucide:file-pen-line" className="h-4 w-4 text-muted" />
          <div className="min-w-0">
            <div className="truncate text-sm font-medium">
              {target.fileName || "Canvas file"}
            </div>
            {target.projectName ? (
              <div className="truncate text-xs text-muted">
                {target.projectName}
              </div>
            ) : null}
          </div>
        </div>
        <Button size="sm" variant="ghost" onPress={() => setShowCatalog(true)}>
          <Icon icon="lucide:list-tree" className="h-4 w-4" />
          Files
        </Button>
      </div>
      <iframe
        key={target.key}
        src={freshCached.url}
        title={target.fileName || `Canvas file ${target.fileId}`}
        allow="clipboard-read; clipboard-write"
        className="min-h-0 flex-1 border-0"
      />
    </div>
  )
}

function CanvasProjectPicker({
  activeTargetKey,
  canClose,
  onClose,
  onSelectFile,
}: {
  activeTargetKey: string | null
  canClose: boolean
  onClose: () => void
  onSelectFile: (project: CanvasProject, file: CanvasProjectFile) => void
}) {
  const catalogQuery = $api.useQuery(
    "get",
    "/v1/canvas/projects",
    {},
    { retry: false }
  )

  if (catalogQuery.isPending) return <CanvasCatalogSkeleton />

  if (catalogQuery.isError) {
    return (
      <DesignState
        icon="lucide:circle-alert"
        title="Canvas files are not available"
        message={errorMessage(
          catalogQuery.error,
          "Could not load Canvas files."
        )}
        action={
          <Button
            size="sm"
            variant="secondary"
            onPress={() => void catalogQuery.refetch()}
          >
            Retry
          </Button>
        }
      />
    )
  }

  const projects = catalogQuery.data?.projects ?? []
  const fileCount = projects.reduce(
    (sum, project) => sum + (project.files?.length ?? 0),
    0
  )
  if (fileCount === 0) {
    return (
      <DesignState
        icon="lucide:file-x"
        title="No Canvas files"
        action={
          <Button
            size="sm"
            variant="secondary"
            onPress={() => void catalogQuery.refetch()}
          >
            Refresh
          </Button>
        }
      />
    )
  }

  return (
    <div className="flex h-full min-h-0 flex-col bg-background">
      <div className="flex h-12 shrink-0 items-center justify-between border-b border-border px-4">
        <div className="flex min-w-0 items-center gap-2">
          <Icon icon="lucide:pen-tool" className="h-4 w-4 text-muted" />
          <div className="truncate text-sm font-medium">Canvas files</div>
          <div className="rounded-full bg-default px-2 py-0.5 text-xs text-muted">
            {fileCount}
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-1">
          <Button
            size="sm"
            variant="ghost"
            isIconOnly
            aria-label="Refresh Canvas files"
            onPress={() => void catalogQuery.refetch()}
          >
            <Icon icon="lucide:refresh-cw" className="h-4 w-4" />
          </Button>
          {canClose ? (
            <Button size="sm" variant="ghost" onPress={onClose}>
              Back
            </Button>
          ) : null}
        </div>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto px-3 py-3">
        <div className="flex flex-col gap-4">
          {projects.map((project, projectIndex) => (
            <div
              key={project.project_id ?? `project-${projectIndex}`}
              className="flex flex-col gap-1.5"
            >
              <div className="flex items-center justify-between px-1">
                <div className="truncate text-xs font-medium tracking-normal text-muted uppercase">
                  {project.name || "Untitled project"}
                </div>
                <div className="text-xs text-muted">
                  {project.files?.length ?? 0}
                </div>
              </div>
              <div className="flex flex-col gap-1">
                {(project.files ?? []).map((file, fileIndex) => {
                  if (!file.file_id) return null
                  const target = canvasCatalogFileTarget(file, project)
                  const active = target.key === activeTargetKey
                  return (
                    <button
                      key={target.key || `file-${fileIndex}`}
                      type="button"
                      className={`group flex min-h-12 w-full items-center gap-3 rounded-lg border px-3 py-2 text-left transition-colors ${
                        active
                          ? "border-foreground/20 bg-default text-foreground"
                          : "border-border bg-surface hover:bg-default"
                      }`}
                      onClick={() => onSelectFile(project, file)}
                    >
                      <Icon
                        icon="lucide:file-pen-line"
                        className="h-4 w-4 shrink-0 text-muted"
                      />
                      <span className="min-w-0 flex-1">
                        <span className="block truncate text-sm font-medium">
                          {file.name || "Untitled file"}
                        </span>
                        {file.page_id ? (
                          <span className="block truncate text-xs text-muted">
                            {file.page_id}
                          </span>
                        ) : null}
                      </span>
                      <Icon
                        icon="lucide:chevron-right"
                        className="h-4 w-4 shrink-0 text-muted opacity-0 transition-opacity group-hover:opacity-100"
                      />
                    </button>
                  )
                })}
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

function CanvasCatalogSkeleton() {
  return (
    <div className="flex h-full min-h-0 flex-col bg-background">
      <div className="flex h-12 shrink-0 items-center border-b border-border px-4">
        <div className="h-4 w-28 animate-pulse rounded bg-default" />
      </div>
      <div className="flex flex-col gap-4 px-3 py-3">
        {[0, 1, 2].map((group) => (
          <div key={group} className="flex flex-col gap-2">
            <div className="h-3 w-32 animate-pulse rounded bg-default" />
            {[0, 1].map((row) => (
              <div
                key={row}
                className="h-12 animate-pulse rounded-lg border border-border bg-surface"
              />
            ))}
          </div>
        ))}
      </div>
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
  message?: string
  action?: ReactNode
}) {
  return (
    <div className="flex h-full items-center justify-center px-6 text-center">
      <div className="flex max-w-sm flex-col items-center gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-lg border border-border bg-background">
          <Icon icon={icon} className="h-5 w-5 text-muted" />
        </div>
        <div className="text-sm font-medium">{title}</div>
        {message ? (
          <p className="text-sm leading-6 text-muted">{message}</p>
        ) : null}
        {action}
      </div>
    </div>
  )
}

function canvasSessionURLEntryFromResponse(data: {
  url?: string
  expires_in?: number
}): CanvasSessionURLEntry {
  if (!data.url || data.expires_in === undefined) {
    throw new Error("Canvas session response was incomplete.")
  }
  const cachedAt = Date.now()
  return {
    url: data.url,
    cachedAt,
    expiresAt: cachedAt + data.expires_in * 1000,
  }
}
