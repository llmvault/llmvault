"use client"

import { useState } from "react"
import type { ReactNode } from "react"
import { Button, Spinner } from "@heroui/react"
import { useMutation } from "@tanstack/react-query"
import { AppIcon } from "@/components/icon"
import {
  canvasArtifactCommentPayload,
  sendCanvasArtifactComment,
  type CanvasArtifact,
  type CanvasArtifactProject,
  type CanvasArtifactType,
  type CanvasViewportMode,
} from "@/app/w/(chat)/_lib/canvas-artifacts"
import { extractErrorMessage as errorMessage } from "@/lib/api/error"

export function CanvasArtifactHeader({
  projects,
  selectedProject,
  artifacts,
  selectedArtifact,
  artifactsPending,
  onRefresh,
  onSelectProject,
  onSelectArtifact,
}: {
  projects: CanvasArtifactProject[]
  selectedProject: CanvasArtifactProject | null
  artifacts: CanvasArtifact[]
  selectedArtifact: CanvasArtifact | null
  artifactsPending: boolean
  onRefresh: () => void
  onSelectProject: (projectId: string) => void
  onSelectArtifact: (artifactId: string) => void
}) {
  return (
    <div className="shrink-0 border-b border-border bg-surface">
      <div className="flex h-12 items-center justify-between gap-3 px-3">
        <div className="flex min-w-0 items-center gap-2">
          <AppIcon icon="panels-top-left" className="h-4 w-4 text-muted" />
          <div className="min-w-0">
            <div className="truncate text-sm font-medium">Canvas</div>
            <div className="truncate text-xs text-muted">
              {selectedProject?.name ?? "Projects"} /{" "}
              {selectedArtifact?.name ?? "Artifacts"}
            </div>
          </div>
        </div>
        <Button
          size="sm"
          variant="ghost"
          isIconOnly
          aria-label="Refresh Canvas"
          onPress={onRefresh}
        >
          <AppIcon icon="refresh-cw" className="h-4 w-4" />
        </Button>
      </div>
      <div className="flex gap-1 overflow-x-auto px-3 pb-2">
        {projects.map((project) => (
          <SelectorTab
            key={project.id}
            selected={project.id === selectedProject?.id}
            icon="folder"
            label={project.name}
            meta={project.artifactCount}
            onClick={() => onSelectProject(project.id)}
          />
        ))}
      </div>
      <div className="flex min-h-11 items-center gap-1 overflow-x-auto border-t border-border px-3 py-2">
        {artifactsPending ? (
          <div className="flex items-center gap-2 text-sm text-muted">
            <Spinner size="sm" aria-label="Loading artifacts" />
            Loading artifacts
          </div>
        ) : artifacts.length ? (
          artifacts.map((artifact) => (
            <SelectorTab
              key={artifact.id}
              selected={artifact.id === selectedArtifact?.id}
              icon={artifactIcon(artifact.type)}
              label={artifact.name}
              caption={artifactTypeLabel(artifact.type)}
              onClick={() => onSelectArtifact(artifact.id)}
            />
          ))
        ) : (
          <div className="text-sm text-muted">
            No artifacts in this project.
          </div>
        )}
      </div>
    </div>
  )
}

function SelectorTab({
  selected,
  icon,
  label,
  caption,
  meta,
  onClick,
}: {
  selected: boolean
  icon: string
  label: string
  caption?: string
  meta?: number
  onClick: () => void
}) {
  return (
    <button
      type="button"
      aria-pressed={selected}
      onClick={onClick}
      className={`flex h-8 shrink-0 items-center gap-1.5 rounded-lg border px-2.5 text-sm transition-colors ${
        selected
          ? "border-foreground/20 bg-default text-foreground"
          : "border-transparent text-muted hover:bg-default hover:text-foreground"
      }`}
    >
      <AppIcon icon={icon} className="h-3.5 w-3.5 shrink-0" />
      <span className="max-w-40 truncate">{label}</span>
      {caption ? <span className="text-xs text-muted">{caption}</span> : null}
      {meta !== undefined ? (
        <span className="rounded-full bg-background px-1.5 py-0.5 text-[11px] text-muted">
          {meta}
        </span>
      ) : null}
    </button>
  )
}

export function CanvasPreviewToolbar({
  artifact,
  viewport,
  onViewportChange,
}: {
  artifact: CanvasArtifact | null
  viewport: CanvasViewportMode
  onViewportChange: (viewport: CanvasViewportMode) => void
}) {
  return (
    <div className="flex h-11 shrink-0 items-center justify-between gap-3 border-b border-border bg-background px-3">
      <div className="flex min-w-0 items-center gap-2">
        <AppIcon
          icon={artifactIcon(artifact?.type)}
          className="h-4 w-4 shrink-0 text-muted"
        />
        <div className="min-w-0 truncate text-sm font-medium">
          {artifact?.name ?? "Select an artifact"}
        </div>
        {artifact ? (
          <span className="shrink-0 rounded-full bg-default px-2 py-0.5 text-xs text-muted">
            {artifactTypeLabel(artifact.type)}
          </span>
        ) : null}
      </div>
      <div className="flex shrink-0 items-center rounded-lg border border-border bg-surface p-0.5">
        {VIEWPORTS.map((item) => (
          <button
            key={item.id}
            type="button"
            aria-label={item.label}
            aria-pressed={viewport === item.id}
            title={item.label}
            onClick={() => onViewportChange(item.id)}
            className={`flex h-7 w-7 items-center justify-center rounded-md transition-colors ${
              viewport === item.id
                ? "bg-default text-foreground"
                : "text-muted hover:text-foreground"
            }`}
          >
            <AppIcon icon={item.icon} className="h-3.5 w-3.5" />
          </button>
        ))}
      </div>
    </div>
  )
}

export function CanvasPreviewPane({
  artifact,
  previewUrl,
  viewport,
  sessionReady,
  loading,
  error,
  onRetry,
}: {
  artifact: CanvasArtifact | null
  previewUrl?: string
  viewport: CanvasViewportMode
  sessionReady: boolean
  loading: boolean
  error: unknown
  onRetry: () => void
}) {
  if (!artifact) {
    return (
      <DesignState
        icon="file-search"
        title="Select an artifact"
        message="Choose a project and artifact to preview."
      />
    )
  }

  if (!sessionReady) {
    return (
      <DesignState
        icon="message-square"
        title="Open a session to preview"
        message="Canvas previews load from the active sandbox."
      />
    )
  }

  if (error && !previewUrl) {
    return (
      <DesignState
        icon="circle-alert"
        title="Preview is not available"
        message={errorMessage(error, "Could not load the Canvas preview.")}
        action={
          <Button size="sm" variant="secondary" onPress={onRetry}>
            Retry
          </Button>
        }
      />
    )
  }

  if (loading && !previewUrl) {
    return (
      <div className="flex min-h-0 flex-1 flex-col gap-4 bg-default/30 px-4 py-5">
        <div className="flex items-center gap-2 text-sm text-muted">
          <Spinner size="sm" aria-label="Opening artifact" />
          Opening artifact
        </div>
        <div className="min-h-0 flex-1 animate-pulse rounded-lg border border-border bg-surface" />
      </div>
    )
  }

  return (
    <div className="min-h-0 flex-1 overflow-auto bg-default/30 p-3">
      <div
        className="mx-auto h-full min-h-[520px] overflow-hidden rounded-lg border border-border bg-white shadow-sm"
        style={{ width: viewportWidth(viewport) }}
      >
        <iframe
          key={`${artifact.id}:${previewUrl}`}
          src={previewUrl}
          title={artifact.name}
          allow="clipboard-read; clipboard-write; fullscreen"
          className="h-full w-full border-0"
        />
      </div>
    </div>
  )
}

export function CanvasCommentComposer({
  sessionId,
  artifact,
  project,
  viewport,
  previewUrl,
}: {
  sessionId: string | null
  artifact: CanvasArtifact | null
  project: CanvasArtifactProject | null
  viewport: CanvasViewportMode
  previewUrl?: string
}) {
  const [body, setBody] = useState("")
  const [selector, setSelector] = useState("")
  const [sent, setSent] = useState(false)
  const [error, setError] = useState("")
  const commentMutation = useMutation({
    mutationFn: (comment: ReturnType<typeof canvasArtifactCommentPayload>) => {
      if (!sessionId) throw new Error("No active session.")
      return sendCanvasArtifactComment(sessionId, comment)
    },
  })
  const disabled = !sessionId || !artifact || commentMutation.isPending
  const trimmedBody = body.trim()

  const submit = async () => {
    if (!artifact || !trimmedBody) return
    setError("")
    setSent(false)
    try {
      await commentMutation.mutateAsync(
        canvasArtifactCommentPayload({
          artifact,
          project,
          viewport,
          body: trimmedBody,
          selector: selector.trim(),
          previewUrl,
        })
      )
      setBody("")
      setSelector("")
      setSent(true)
    } catch (cause) {
      setError(errorMessage(cause, "Could not send artifact comment."))
    }
  }

  return (
    <aside className="flex shrink-0 flex-col border-t border-border bg-surface xl:w-80 xl:border-t-0 xl:border-l">
      <div className="border-b border-border px-3 py-2">
        <div className="flex items-center gap-2 text-sm font-medium">
          <AppIcon icon="message-square-plus" className="h-4 w-4" />
          Comment
        </div>
      </div>
      <div className="flex flex-col gap-2 p-3">
        <input
          type="text"
          value={selector}
          disabled={disabled}
          placeholder="data-canvas-id or selector"
          onChange={(event) => {
            setSelector(event.target.value)
            setSent(false)
          }}
          className="h-9 rounded-lg border border-border bg-background px-3 text-sm transition-colors outline-none placeholder:text-muted focus:border-foreground/30 disabled:cursor-not-allowed disabled:opacity-50"
        />
        <textarea
          value={body}
          disabled={disabled}
          placeholder="Leave a comment for the agent"
          rows={5}
          onChange={(event) => {
            setBody(event.target.value)
            setSent(false)
          }}
          className="min-h-28 resize-none rounded-lg border border-border bg-background px-3 py-2 text-sm leading-5 transition-colors outline-none placeholder:text-muted focus:border-foreground/30 disabled:cursor-not-allowed disabled:opacity-50"
        />
        <div className="flex min-h-8 items-center justify-between gap-3">
          <div className="min-w-0 flex-1 text-xs">
            {error ? (
              <span className="text-danger">{error}</span>
            ) : sent ? (
              <span className="text-success">Sent</span>
            ) : !sessionId ? (
              <span className="text-muted">No active session</span>
            ) : null}
          </div>
          <Button
            size="sm"
            variant="secondary"
            isDisabled={disabled || !trimmedBody}
            onPress={() => void submit()}
          >
            {commentMutation.isPending ? (
              <AppIcon
                icon="loader-circle"
                className="h-4 w-4 animate-spin"
              />
            ) : (
              <AppIcon icon="send" className="h-4 w-4" />
            )}
            Send
          </Button>
        </div>
      </div>
    </aside>
  )
}

export function CanvasBrowserSkeleton() {
  return (
    <div className="flex h-full min-h-0 flex-col bg-background">
      <div className="shrink-0 border-b border-border px-3 py-3">
        <div className="h-4 w-28 animate-pulse rounded bg-default" />
        <div className="mt-3 flex gap-2">
          {[0, 1, 2].map((item) => (
            <div
              key={item}
              className="h-8 w-28 animate-pulse rounded-lg bg-default"
            />
          ))}
        </div>
      </div>
      <div className="flex min-h-0 flex-1 gap-3 p-3">
        <div className="min-h-0 flex-1 animate-pulse rounded-lg border border-border bg-surface" />
        <div className="hidden w-80 animate-pulse rounded-lg border border-border bg-surface xl:block" />
      </div>
    </div>
  )
}

export function DesignState({
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
          <AppIcon icon={icon} className="h-5 w-5 text-muted" />
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

const VIEWPORTS: {
  id: CanvasViewportMode
  label: string
  icon: string
}[] = [
  { id: "desktop", label: "Desktop", icon: "monitor" },
  { id: "tablet", label: "Tablet", icon: "tablet" },
  { id: "mobile", label: "Mobile", icon: "smartphone" },
]

function viewportWidth(viewport: CanvasViewportMode) {
  switch (viewport) {
    case "mobile":
      return "min(100%, 390px)"
    case "tablet":
      return "min(100%, 768px)"
    default:
      return "100%"
  }
}

function artifactIcon(type?: CanvasArtifactType) {
  return type === "presentation" ? "presentation" : "layout"
}

function artifactTypeLabel(type: CanvasArtifactType) {
  switch (type) {
    case "presentation":
      return "Slides"
    case "web_page":
      return "Page"
    default:
      return type
        .split(/[_\s-]+/)
        .filter(Boolean)
        .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
        .join(" ")
  }
}
