"use client"

import { useMemo, useState } from "react"
import { Button } from "@heroui/react"
import {
  useCanvasArtifact,
  useCanvasArtifactPreviewURL,
  useCanvasProjects,
  type CanvasViewportMode,
} from "@/app/w/(chat)/_lib/canvas-artifacts"
import { usePanelArtifactTargetStore } from "@/app/w/(chat)/_stores/panel-artifact-target-store"
import { extractErrorMessage as errorMessage } from "@/lib/api/error"
import {
  CanvasArtifactHeader,
  CanvasBrowserSkeleton,
  CanvasCommentComposer,
  CanvasPreviewPane,
  CanvasPreviewToolbar,
  DesignState,
} from "./design-canvas-components"

export function DesignView({ sessionId = "new-chat" }: { sessionId?: string }) {
  const activeSessionId = sessionId === "new-chat" ? null : sessionId
  const [selectedProjectId, setSelectedProjectId] = useState<string | null>(
    null
  )
  const [selectedArtifactId, setSelectedArtifactId] = useState<string | null>(
    null
  )
  const [viewport, setViewport] = useState<CanvasViewportMode>("desktop")

  // Target selection (from an artifact.synced notice) wins until the user picks
  // an artifact themselves, at which point the target is cleared and local
  // selection takes over.
  const artifactTarget = usePanelArtifactTargetStore((state) =>
    state.target?.sessionId === sessionId ? state.target.artifactId : null
  )
  const clearArtifactTarget = usePanelArtifactTargetStore(
    (state) => state.clear
  )
  const activeArtifactId = artifactTarget ?? selectedArtifactId
  const selectArtifact = (artifactId: string | null) => {
    setSelectedArtifactId(artifactId)
    clearArtifactTarget()
  }

  const projectsQuery = useCanvasProjects(activeSessionId)
  const projects = useMemo(() => projectsQuery.data ?? [], [projectsQuery.data])
  const selectedProject = useMemo(
    () =>
      projects.find((project) => project.id === selectedProjectId) ??
      projects[0] ??
      null,
    [projects, selectedProjectId]
  )

  const artifacts = useMemo(
    () => selectedProject?.artifacts ?? [],
    [selectedProject?.artifacts]
  )
  const listedArtifact = useMemo(
    () =>
      artifacts.find((artifact) => artifact.id === activeArtifactId) ??
      artifacts[0] ??
      null,
    [artifacts, activeArtifactId]
  )

  const detailQuery = useCanvasArtifact(listedArtifact?.id)
  const activeArtifact = detailQuery.data ?? listedArtifact
  const previewQuery = useCanvasArtifactPreviewURL({
    artifactId: activeArtifact?.id,
    sessionId: activeSessionId,
  })

  if (projectsQuery.isPending) return <CanvasBrowserSkeleton />

  if (projectsQuery.isError) {
    return (
      <DesignState
        icon="circle-alert"
        title="Canvas is not available"
        message={errorMessage(
          projectsQuery.error,
          "Could not load Canvas projects."
        )}
        action={
          <Button
            size="sm"
            variant="secondary"
            onPress={() => void projectsQuery.refetch()}
          >
            Retry
          </Button>
        }
      />
    )
  }

  if (!projects.length) {
    return (
      <DesignState
        icon="panels-top-left"
        title="No Canvas projects"
        action={
          <Button
            size="sm"
            variant="secondary"
            onPress={() => void projectsQuery.refetch()}
          >
            Refresh
          </Button>
        }
      />
    )
  }

  return (
    <div className="flex h-full min-h-0 flex-col bg-background">
      <CanvasArtifactHeader
        projects={projects}
        selectedProject={selectedProject}
        artifacts={artifacts}
        selectedArtifact={activeArtifact}
        artifactsPending={projectsQuery.isPending}
        onRefresh={() => {
          void projectsQuery.refetch()
          void detailQuery.refetch()
          void previewQuery.refetch()
        }}
        onSelectProject={(projectId) => {
          setSelectedProjectId(projectId)
          selectArtifact(null)
        }}
        onSelectArtifact={selectArtifact}
      />
      <div className="flex min-h-0 flex-1 flex-col xl:flex-row">
        <div className="flex min-h-0 flex-1 flex-col">
          <CanvasPreviewToolbar
            artifact={activeArtifact}
            viewport={viewport}
            onViewportChange={setViewport}
          />
          <CanvasPreviewPane
            artifact={activeArtifact}
            previewUrl={previewQuery.data?.url}
            viewport={viewport}
            sessionReady={Boolean(activeSessionId)}
            loading={projectsQuery.isPending || previewQuery.isPending}
            error={detailQuery.error ?? previewQuery.error ?? null}
            onRetry={() => {
              void projectsQuery.refetch()
              void detailQuery.refetch()
              void previewQuery.refetch()
            }}
          />
        </div>
        <CanvasCommentComposer
          key={activeArtifact?.id ?? "no-artifact"}
          sessionId={activeSessionId}
          artifact={activeArtifact}
          project={selectedProject}
          viewport={viewport}
          previewUrl={previewQuery.data?.url}
        />
      </div>
    </div>
  )
}
