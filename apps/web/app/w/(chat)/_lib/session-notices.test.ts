import type { QueryClient } from "@tanstack/react-query"
import { afterEach, describe, expect, it, vi, type Mock } from "vitest"
import { queryKeys } from "@/lib/api/query-keys"
import { chatQueryKeys } from "@/app/w/(chat)/_lib/chat-cache"
import {
  handleSessionNotice,
  runSessionNoticeCatchUp,
  type SessionNotice,
} from "@/app/w/(chat)/_lib/session-notices"
import { useSessionWorkspaceStore } from "@/app/w/(chat)/_stores/session-workspace-store"
import { usePanelArtifactTargetStore } from "@/app/w/(chat)/_stores/panel-artifact-target-store"

const SESSION_ID = "session-1"

function mockQueryClient() {
  const invalidateQueries = vi.fn()
  return {
    client: { invalidateQueries } as unknown as QueryClient,
    invalidateQueries: invalidateQueries as Mock,
  }
}

function invalidatedKeys(invalidateQueries: Mock) {
  return invalidateQueries.mock.calls.map((call) => call[0]?.queryKey)
}

function notice(overrides: Partial<SessionNotice>): SessionNotice {
  return {
    type: "artifact.synced",
    org_id: "org-1",
    data: {},
    published_at: "2026-07-09T00:00:00Z",
    ...overrides,
  }
}

describe("handleSessionNotice", () => {
  afterEach(() => {
    usePanelArtifactTargetStore.setState({ target: null })
    useSessionWorkspaceStore.setState({ workspaces: {} })
    vi.clearAllMocks()
  })

  it("artifact.synced invalidates canvas queries, targets the artifact, and opens the panel", () => {
    const { client, invalidateQueries } = mockQueryClient()

    handleSessionNotice(
      client,
      SESSION_ID,
      notice({ type: "artifact.synced", data: { artifact_id: "artifact-9" } })
    )

    const keys = invalidatedKeys(invalidateQueries)
    expect(keys).toContainEqual(queryKeys.canvasProjects())
    expect(keys).toContainEqual(queryKeys.canvasArtifacts())
    expect(keys).toContainEqual([
      ...queryKeys.canvasArtifact(),
      { params: { path: { artifactID: "artifact-9" } } },
    ])

    expect(usePanelArtifactTargetStore.getState().target).toEqual({
      sessionId: SESSION_ID,
      artifactId: "artifact-9",
    })

    const panel =
      useSessionWorkspaceStore.getState().workspaces[SESSION_ID]?.rightPanel
    expect(panel?.openViews).toContain("design")
    expect(panel?.activeView).toBe("design")
  })

  it("artifact.synced does not steal focus when design is already open", () => {
    const { client } = mockQueryClient()
    const store = useSessionWorkspaceStore.getState()
    store.openPanelView(SESSION_ID, "design")
    store.setActivePanelView(SESSION_ID, "review")

    handleSessionNotice(
      client,
      SESSION_ID,
      notice({ type: "artifact.synced", data: { artifact_id: "artifact-9" } })
    )

    const panel =
      useSessionWorkspaceStore.getState().workspaces[SESSION_ID]?.rightPanel
    expect(panel?.activeView).toBe("review")
    expect(usePanelArtifactTargetStore.getState().target?.artifactId).toBe(
      "artifact-9"
    )
  })

  it("usage.updated invalidates the session-usage query", () => {
    const { client, invalidateQueries } = mockQueryClient()

    handleSessionNotice(
      client,
      SESSION_ID,
      notice({ type: "usage.updated", data: { session_id: SESSION_ID } })
    )

    const keys = invalidatedKeys(invalidateQueries)
    expect(keys).toContainEqual(queryKeys.sessionUsage(SESSION_ID))
    expect(usePanelArtifactTargetStore.getState().target).toBeNull()
  })

  it("session.events.appended invalidates the session-events query", () => {
    const { client, invalidateQueries } = mockQueryClient()

    handleSessionNotice(
      client,
      SESSION_ID,
      notice({
        type: "session.events.appended",
        data: { event_id: "event-9", event_at: "2026-07-09T00:00:00Z" },
      })
    )

    const keys = invalidatedKeys(invalidateQueries)
    expect(keys).toContainEqual(chatQueryKeys.sessionEvents(SESSION_ID))
    expect(usePanelArtifactTargetStore.getState().target).toBeNull()
  })

  it("catch-up invalidates the session-events query", () => {
    const { client, invalidateQueries } = mockQueryClient()

    runSessionNoticeCatchUp(client, SESSION_ID)

    const keys = invalidatedKeys(invalidateQueries)
    expect(keys).toContainEqual(chatQueryKeys.sessionEvents(SESSION_ID))
  })

  it("ignores unknown notice types without touching queries or stores", () => {
    const { client, invalidateQueries } = mockQueryClient()

    handleSessionNotice(
      client,
      SESSION_ID,
      notice({ type: "something.new", data: { anything: true } })
    )

    expect(invalidateQueries).not.toHaveBeenCalled()
    expect(usePanelArtifactTargetStore.getState().target).toBeNull()
    expect(
      useSessionWorkspaceStore.getState().workspaces[SESSION_ID]
    ).toBeUndefined()
  })
})
