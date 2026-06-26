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

  it("tracks multiple Canvas targets and cached session URLs", () => {
    const store = useSessionWorkspaceStore.getState()
    const first = {
      key: "file-a",
      fileId: "file-a",
      sourceUrl: "https://canvas.usehivy.com/#/workspace?file-id=file-a",
    }
    const second = {
      key: "file-b:page-b",
      fileId: "file-b",
      pageId: "page-b",
      sourceUrl:
        "https://canvas.usehivy.com/#/workspace?file-id=file-b&page-id=page-b",
    }

    store.openCanvasTarget("session-a", first)
    store.openCanvasTarget("session-a", second)
    store.setCanvasSessionURL("session-a", second.key, {
      url: "https://canvas.usehivy.com/api/hivy/session?token=abc",
      cachedAt: 1_000,
      expiresAt: 2_000,
    })

    const workspace = selectSessionWorkspace(
      useSessionWorkspaceStore.getState(),
      "session-a"
    )
    expect(workspace.rightPanel).toMatchObject({
      open: true,
      openViews: ["design"],
      activeView: "design",
    })
    expect(workspace.canvas.targets).toEqual([first, second])
    expect(workspace.canvas.activeTargetKey).toBe(second.key)
    expect(workspace.canvas.sessionURLs[second.key]?.url).toContain(
      "/api/hivy/session"
    )
  })

  it("opens preview URLs in the browser panel", () => {
    const store = useSessionWorkspaceStore.getState()
    const url = "https://5173-um7j159u.preview.usehivy.com/"

    store.openPanelView("session-a", "terminal")
    store.openBrowserURL("session-a", url)

    const workspace = selectSessionWorkspace(
      useSessionWorkspaceStore.getState(),
      "session-a"
    )
    expect(workspace.rightPanel).toMatchObject({
      open: true,
      openViews: ["terminal", "browser"],
      activeView: "browser",
    })
    expect(workspace.browser).toMatchObject({
      url,
      src: url,
      reloadKey: 1,
    })
  })

  it("opens selected subagent runs in the subagents panel", () => {
    const store = useSessionWorkspaceStore.getState()

    store.openPanelView("session-a", "terminal")
    store.openSubagentRun("session-a", "job-1")

    const workspace = selectSessionWorkspace(
      useSessionWorkspaceStore.getState(),
      "session-a"
    )
    expect(workspace.rightPanel).toMatchObject({
      open: true,
      openViews: ["terminal", "side-chat"],
      activeView: "side-chat",
    })
    expect(workspace.subagents.activeJobId).toBe("job-1")
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
