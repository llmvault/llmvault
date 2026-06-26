"use client"

import { useMemo, useState } from "react"
import { Button } from "@heroui/react"
import { useQuery } from "@tanstack/react-query"
import {
  fetchCanvasArtifact,
  fetchCanvasArtifactPreviewURL,
  fetchCanvasArtifacts,
  fetchCanvasProjects,
  type CanvasViewportMode,
} from "@/app/w/(chat)/_lib/canvas-artifacts"
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

  const projectsQuery = useQuery({
    queryKey: ["canvas-artifact-projects"],
    queryFn: ({ signal }) => fetchCanvasProjects(signal),
    retry: false,
  })
  const projects = useMemo(() => projectsQuery.data ?? [], [projectsQuery.data])
  const selectedProject = useMemo(
    () =>
      projects.find((project) => project.id === selectedProjectId) ??
      projects[0] ??
      null,
    [projects, selectedProjectId]
  )

  const artifactsQuery = useQuery({
    enabled: Boolean(selectedProject?.id),
    queryKey: ["canvas-artifacts", selectedProject?.id, activeSessionId],
    queryFn: ({ signal }) =>
      fetchCanvasArtifacts(
        {
          projectId: selectedProject?.id,
          sessionId: activeSessionId,
        },
        signal
      ),
    retry: false,
  })
  const artifacts = useMemo(
    () => artifactsQuery.data ?? [],
    [artifactsQuery.data]
  )
  const listedArtifact = useMemo(
    () =>
      artifacts.find((artifact) => artifact.id === selectedArtifactId) ??
      artifacts[0] ??
      null,
    [artifacts, selectedArtifactId]
  )

  const detailQuery = useQuery({
    enabled: Boolean(listedArtifact?.id),
    queryKey: ["canvas-artifact", listedArtifact?.id],
    queryFn: ({ signal }) =>
      fetchCanvasArtifact(listedArtifact?.id ?? "", signal),
    retry: false,
  })
  const activeArtifact = detailQuery.data ?? listedArtifact
  const previewQuery = useQuery({
    enabled: Boolean(activeArtifact?.id && activeSessionId),
    queryKey: ["canvas-artifact-preview", activeArtifact?.id, activeSessionId],
    queryFn: ({ signal }) =>
      fetchCanvasArtifactPreviewURL(
        {
          artifactId: activeArtifact?.id ?? "",
          sessionId: activeSessionId,
        },
        signal
      ),
    retry: false,
  })

  if (projectsQuery.isPending) return <CanvasBrowserSkeleton />

  if (projectsQuery.isError) {
    return (
      <DesignState
        icon="lucide:circle-alert"
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
        icon="lucide:panels-top-left"
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
        artifactsPending={artifactsQuery.isPending}
        onRefresh={() => {
          void projectsQuery.refetch()
          void artifactsQuery.refetch()
          void detailQuery.refetch()
          void previewQuery.refetch()
        }}
        onSelectProject={(projectId) => {
          setSelectedProjectId(projectId)
          setSelectedArtifactId(null)
        }}
        onSelectArtifact={setSelectedArtifactId}
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
            loading={artifactsQuery.isPending || previewQuery.isPending}
            error={
              artifactsQuery.error ??
              detailQuery.error ??
              previewQuery.error ??
              null
            }
            onRetry={() => {
              void artifactsQuery.refetch()
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
