import { fetchEventSource } from "@microsoft/fetch-event-source"
import type { QueryClient } from "@tanstack/react-query"
import { queryKeys } from "@/lib/api/query-keys"
import { chatQueryKeys } from "@/app/w/(chat)/_lib/chat-cache"
import { useSessionWorkspaceStore } from "@/app/w/(chat)/_stores/session-workspace-store"
import { usePanelArtifactTargetStore } from "@/app/w/(chat)/_stores/panel-artifact-target-store"

export interface SessionNotice {
  type: string
  org_id: string
  session_id?: string
  data: Record<string, unknown>
  published_at: string
}

class NoticeStreamError extends Error {
  constructor(readonly status: number) {
    super(`Session notice stream failed with HTTP ${status}`)
    this.name = "NoticeStreamError"
  }
}

class NoticeStreamClosedError extends Error {
  constructor() {
    super("session notice stream closed")
    this.name = "NoticeStreamClosedError"
  }
}

export function noticeStreamHTTPStatus(error: unknown): number | undefined {
  return error instanceof NoticeStreamError ? error.status : undefined
}

interface SubscribeSessionNoticesOptions {
  signal: AbortSignal
  onOpen?: () => void
  onNotice: (notice: SessionNotice) => void
}

export async function subscribeToSessionNotices(
  sessionId: string,
  { signal, onOpen, onNotice }: SubscribeSessionNoticesOptions
) {
  await fetchEventSource(
    `/api/proxy/v1/sessions/${encodeURIComponent(sessionId)}/notices`,
    {
      method: "GET",
      headers: { Accept: "text/event-stream" },
      credentials: "include",
      openWhenHidden: true,
      signal,
      async onopen(response) {
        const contentType = response.headers.get("content-type") ?? ""
        if (!response.ok || !contentType.includes("text/event-stream")) {
          throw new NoticeStreamError(response.status)
        }
        onOpen?.()
      },
      onmessage(message) {
        if (message.event !== "session.notice") return
        const notice = parseSessionNotice(message.data)
        if (notice) onNotice(notice)
      },
      onclose() {
        throw new NoticeStreamClosedError()
      },
      onerror(error) {
        throw error
      },
    }
  )
}

export function handleSessionNotice(
  queryClient: QueryClient,
  sessionId: string,
  notice: SessionNotice
) {
  switch (notice.type) {
    case "artifact.synced": {
      const artifactID =
        typeof notice.data.artifact_id === "string"
          ? notice.data.artifact_id
          : ""
      void queryClient.invalidateQueries({
        queryKey: queryKeys.canvasProjects(),
      })
      void queryClient.invalidateQueries({
        queryKey: queryKeys.canvasArtifacts(),
      })
      if (artifactID) {
        void queryClient.invalidateQueries({
          queryKey: [
            ...queryKeys.canvasArtifact(),
            { params: { path: { artifactID } } },
          ],
        })
      }
      openSyncedArtifact(sessionId, artifactID)
      break
    }
    case "usage.updated":
      void queryClient.invalidateQueries({
        queryKey: queryKeys.sessionUsage(sessionId),
      })
      break
    case "session.events.appended":
      void queryClient.invalidateQueries({
        queryKey: chatQueryKeys.sessionEvents(sessionId),
      })
      break
  }
}

export function runSessionNoticeCatchUp(
  queryClient: QueryClient,
  sessionId: string
) {
  void queryClient.invalidateQueries({ queryKey: queryKeys.canvasProjects() })
  void queryClient.invalidateQueries({ queryKey: queryKeys.canvasArtifacts() })
  void queryClient.invalidateQueries({ queryKey: queryKeys.canvasArtifact() })
  void queryClient.invalidateQueries({
    queryKey: queryKeys.sessionUsage(sessionId),
  })
  void queryClient.invalidateQueries({
    queryKey: chatQueryKeys.sessionEvents(sessionId),
  })
}

function openSyncedArtifact(sessionId: string, artifactID: string) {
  const workspace = useSessionWorkspaceStore.getState()
  const openViews =
    workspace.workspaces[sessionId]?.rightPanel.openViews ?? []
  if (!openViews.includes("design")) {
    workspace.openPanelView(sessionId, "design")
  }
  if (artifactID) {
    usePanelArtifactTargetStore.getState().openArtifact(sessionId, artifactID)
  }
}

function parseSessionNotice(raw: string): SessionNotice | null {
  if (!raw) return null
  try {
    const parsed = JSON.parse(raw) as unknown
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      return null
    }
    const record = parsed as Record<string, unknown>
    if (typeof record.type !== "string" || typeof record.org_id !== "string") {
      return null
    }
    const data = record.data
    return {
      type: record.type,
      org_id: record.org_id,
      session_id:
        typeof record.session_id === "string" ? record.session_id : undefined,
      data:
        data && typeof data === "object" && !Array.isArray(data)
          ? (data as Record<string, unknown>)
          : {},
      published_at:
        typeof record.published_at === "string" ? record.published_at : "",
    }
  } catch {
    return null
  }
}
