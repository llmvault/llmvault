import { afterEach, describe, expect, it } from "vitest"
import {
  selectSessionWorkspace,
  useSessionWorkspaceStore,
} from "@/app/w/(chat)/_stores/session-workspace-store"

describe("session workspace store", () => {
  afterEach(() => {
    useSessionWorkspaceStore.setState({
      scopeKey: "test",
      status: "idle",
      workspaces: {},
    })
  })

  it("returns a stable fallback workspace for untouched sessions", () => {
    const state = useSessionWorkspaceStore.getState()

    expect(selectSessionWorkspace(state, "missing-session")).toBe(
      selectSessionWorkspace(state, "missing-session")
    )
  })

  it("keeps composer drafts per session", () => {
    const store = useSessionWorkspaceStore.getState()

    store.setComposerText("session-a", "draft A")
    store.setComposerText("session-b", "draft B")

    const state = useSessionWorkspaceStore.getState()
    expect(selectSessionWorkspace(state, "session-a").composer.text).toBe(
      "draft A"
    )
    expect(selectSessionWorkspace(state, "session-b").composer.text).toBe(
      "draft B"
    )
  })

  it("tracks right panel views per session", () => {
    const store = useSessionWorkspaceStore.getState()

    store.openPanelView("session-a", "review")
    store.openPanelView("session-b", "browser")

    const state = useSessionWorkspaceStore.getState()
    expect(selectSessionWorkspace(state, "session-a").rightPanel).toMatchObject(
      {
        open: true,
        openViews: ["review"],
        activeView: "review",
      }
    )
    expect(selectSessionWorkspace(state, "session-b").rightPanel).toMatchObject(
      {
        open: true,
        openViews: ["browser"],
        activeView: "browser",
      }
    )
  })

  it("keeps the right panel open when the last view is closed", () => {
    const store = useSessionWorkspaceStore.getState()

    store.openPanelView("session-a", "review")
    store.closePanelView("session-a", "review")

    expect(
      selectSessionWorkspace(useSessionWorkspaceStore.getState(), "session-a")
        .rightPanel
    ).toMatchObject({
      open: true,
      openViews: [],
      activeView: null,
    })
  })

  it("persists pending line comments in the session workspace", () => {
    useSessionWorkspaceStore.getState().addLineComment("session-a", {
      id: "comment-1",
      sourceKey: "repo:file.ts",
      sourceKind: "review",
      path: "file.ts",
      displayPath: "file.ts",
      lineNumber: 12,
      body: "Check this",
      createdAt: 1781900000000,
    })

    expect(
      selectSessionWorkspace(useSessionWorkspaceStore.getState(), "session-a")
        .lineComments
    ).toEqual([
      expect.objectContaining({
        id: "comment-1",
        body: "Check this",
      }),
    ])
  })
})
