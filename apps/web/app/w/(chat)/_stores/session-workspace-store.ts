"use client"

import { useEffect } from "react"
import { create } from "zustand"
import { subscribeWithSelector } from "zustand/middleware"
import type { AttachmentDescriptionState } from "@/app/w/(chat)/_components/composer-attachments"
import type { CodeLineComment } from "@/app/w/(chat)/_components/line-comments"
import type {
  WorkspaceRepoTreeCache,
  WorkspaceUploadItem,
} from "./session-workspace-types"
import {
  DEFAULT_SESSION_WORKSPACE,
  EMPTY_WORKSPACES,
  PERSIST_DEBOUNCE_MS,
  clearWorkspacePersistence,
  createDefaultSessionWorkspace,
  loadPersistedWorkspaces,
  persistWorkspaceState,
  pruneStoredBlobs,
  workspaceScopeKey,
} from "./session-workspace-persistence"

export {
  deleteDraftAttachmentBlob,
  readDraftAttachmentBlob,
  storeDraftAttachmentBlob,
} from "./session-workspace-persistence"

export type {
  WorkspaceRepoTreeCache,
  WorkspaceUploadItem,
} from "./session-workspace-types"

type SessionWorkspaceStatus = "idle" | "hydrating" | "ready"

export type WorkspacePanelViewID =
  | "review"
  | "terminal"
  | "browser"
  | "files"
  | "design"
  | "sheets"
  | "apps"
  | "side-chat"

export interface SessionWorkspace {
  lastTouchedAt: number
  composer: {
    text: string
    effort: string
    uploads: WorkspaceUploadItem[]
    attachmentDescriptions: Record<string, AttachmentDescriptionState>
  }
  lineComments: CodeLineComment[]
  rightPanel: {
    open: boolean
    openViews: WorkspacePanelViewID[]
    activeView: WorkspacePanelViewID | null
    maximized: boolean
    sizePercent: number
  }
  files: {
    selectedRepoId: string | null
    selectedFile: { path: string; repoId: string } | null
    repoTreeCaches: Record<string, WorkspaceRepoTreeCache>
  }
  browser: {
    url: string
    src: string
    reloadKey: number
  }
  review: {
    diffStyle: "unified" | "split"
  }
  terminal: {
    cwd?: string
  }
  subagents: {
    activeJobId: string | null
  }
  scroll: {
    anchor?: string
    top?: number
  }
}

interface WorkspaceScope {
  orgId?: string | null
  userId?: string | null
}

interface SessionWorkspaceStoreState {
  scopeKey: string
  status: SessionWorkspaceStatus
  workspaces: Record<string, SessionWorkspace>
  setScope: (scope: WorkspaceScope) => void
  touchWorkspace: (sessionId: string) => void
  setComposerText: (sessionId: string, text: string) => void
  setComposerEffort: (sessionId: string, effort: string) => void
  setComposerUploads: (
    sessionId: string,
    update: (uploads: WorkspaceUploadItem[]) => WorkspaceUploadItem[]
  ) => void
  setAttachmentDescriptions: (
    sessionId: string,
    update: (
      descriptions: Record<string, AttachmentDescriptionState>
    ) => Record<string, AttachmentDescriptionState>
  ) => void
  clearComposerAfterSend: (sessionId: string) => void
  setLineComments: (sessionId: string, comments: CodeLineComment[]) => void
  addLineComment: (sessionId: string, comment: CodeLineComment) => void
  removeLineComment: (sessionId: string, id: string) => void
  removeLineComments: (sessionId: string, ids: string[]) => void
  clearLineComments: (sessionId: string) => void
  openPanelView: (sessionId: string, view: WorkspacePanelViewID) => void
  closePanelView: (sessionId: string, view: WorkspacePanelViewID) => void
  setActivePanelView: (
    sessionId: string,
    view: WorkspacePanelViewID | null
  ) => void
  setRightPanelOpen: (sessionId: string, open: boolean) => void
  setRightPanelMaximized: (sessionId: string, maximized: boolean) => void
  setRightPanelSize: (sessionId: string, sizePercent: number) => void
  setFilesSelectedRepo: (sessionId: string, repoId: string | null) => void
  setFilesSelectedFile: (
    sessionId: string,
    file: { path: string; repoId: string } | null
  ) => void
  setFilesRepoTreeCaches: (
    sessionId: string,
    update: (
      caches: Record<string, WorkspaceRepoTreeCache>
    ) => Record<string, WorkspaceRepoTreeCache>
  ) => void
  setBrowserURL: (sessionId: string, url: string) => void
  navigateBrowser: (sessionId: string, src: string) => void
  openBrowserURL: (sessionId: string, url: string, src?: string) => void
  reloadBrowser: (sessionId: string) => void
  openSubagentRun: (sessionId: string, jobId: string) => void
  setReviewDiffStyle: (
    sessionId: string,
    diffStyle: SessionWorkspace["review"]["diffStyle"]
  ) => void
}

let hydrationRun = 0
let persistTimer: ReturnType<typeof setTimeout> | null = null

export const useSessionWorkspaceStore = create<SessionWorkspaceStoreState>()(
  subscribeWithSelector((setState, getState) => ({
    scopeKey: "anonymous",
    status: "idle",
    workspaces: EMPTY_WORKSPACES,
    setScope(scope) {
      const nextScopeKey = workspaceScopeKey(scope)
      const current = getState()
      if (current.scopeKey === nextScopeKey && current.status === "ready") {
        return
      }
      const run = hydrationRun + 1
      hydrationRun = run
      setState({ scopeKey: nextScopeKey, status: "hydrating" })
      void loadPersistedWorkspaces(nextScopeKey).then((workspaces) => {
        if (hydrationRun !== run) return
        setState({
          scopeKey: nextScopeKey,
          status: "ready",
          workspaces,
        })
      })
    },
    touchWorkspace(sessionId) {
      setState((state) =>
        updateWorkspaceState(state, sessionId, (workspace) => workspace)
      )
    },
    setComposerText(sessionId, text) {
      setState((state) =>
        updateWorkspaceState(state, sessionId, (workspace) => ({
          ...workspace,
          composer: { ...workspace.composer, text },
        }))
      )
    },
    setComposerEffort(sessionId, effort) {
      setState((state) =>
        updateWorkspaceState(state, sessionId, (workspace) => ({
          ...workspace,
          composer: { ...workspace.composer, effort },
        }))
      )
    },
    setComposerUploads(sessionId, update) {
      setState((state) =>
        updateWorkspaceState(state, sessionId, (workspace) => ({
          ...workspace,
          composer: {
            ...workspace.composer,
            uploads: update(workspace.composer.uploads),
          },
        }))
      )
    },
    setAttachmentDescriptions(sessionId, update) {
      setState((state) =>
        updateWorkspaceState(state, sessionId, (workspace) => ({
          ...workspace,
          composer: {
            ...workspace.composer,
            attachmentDescriptions: update(
              workspace.composer.attachmentDescriptions
            ),
          },
        }))
      )
    },
    clearComposerAfterSend(sessionId) {
      setState((state) =>
        updateWorkspaceState(state, sessionId, (workspace) => ({
          ...workspace,
          composer: {
            ...workspace.composer,
            text: "",
            uploads: [],
            attachmentDescriptions: {},
          },
        }))
      )
    },
    setLineComments(sessionId, comments) {
      setState((state) =>
        updateWorkspaceState(state, sessionId, (workspace) => ({
          ...workspace,
          lineComments: comments,
        }))
      )
    },
    addLineComment(sessionId, comment) {
      setState((state) =>
        updateWorkspaceState(state, sessionId, (workspace) => ({
          ...workspace,
          lineComments: [...workspace.lineComments, comment],
        }))
      )
    },
    removeLineComment(sessionId, id) {
      setState((state) =>
        updateWorkspaceState(state, sessionId, (workspace) => ({
          ...workspace,
          lineComments: workspace.lineComments.filter(
            (comment) => comment.id !== id
          ),
        }))
      )
    },
    removeLineComments(sessionId, ids) {
      if (!ids.length) return
      const removeSet = new Set(ids)
      setState((state) =>
        updateWorkspaceState(state, sessionId, (workspace) => ({
          ...workspace,
          lineComments: workspace.lineComments.filter(
            (comment) => !removeSet.has(comment.id)
          ),
        }))
      )
    },
    clearLineComments(sessionId) {
      setState((state) =>
        updateWorkspaceState(state, sessionId, (workspace) => ({
          ...workspace,
          lineComments: [],
        }))
      )
    },
    openPanelView(sessionId, view) {
      setState((state) =>
        updateWorkspaceState(state, sessionId, (workspace) => ({
          ...workspace,
          rightPanel: {
            ...workspace.rightPanel,
            open: true,
            openViews: workspace.rightPanel.openViews.includes(view)
              ? workspace.rightPanel.openViews
              : [...workspace.rightPanel.openViews, view],
            activeView: view,
          },
        }))
      )
    },
    closePanelView(sessionId, view) {
      setState((state) =>
        updateWorkspaceState(state, sessionId, (workspace) => {
          const openViews = workspace.rightPanel.openViews.filter(
            (entry) => entry !== view
          )
          return {
            ...workspace,
            rightPanel: {
              ...workspace.rightPanel,
              openViews,
              activeView:
                workspace.rightPanel.activeView === view
                  ? (openViews[openViews.length - 1] ?? null)
                  : workspace.rightPanel.activeView,
            },
          }
        })
      )
    },
    setActivePanelView(sessionId, view) {
      setState((state) =>
        updateWorkspaceState(state, sessionId, (workspace) => ({
          ...workspace,
          rightPanel: {
            ...workspace.rightPanel,
            open: view ? true : workspace.rightPanel.open,
            activeView: view,
          },
        }))
      )
    },
    setRightPanelOpen(sessionId, open) {
      setState((state) =>
        updateWorkspaceState(state, sessionId, (workspace) => ({
          ...workspace,
          rightPanel: {
            ...workspace.rightPanel,
            open,
            maximized: open ? workspace.rightPanel.maximized : false,
          },
        }))
      )
    },
    setRightPanelMaximized(sessionId, maximized) {
      setState((state) =>
        updateWorkspaceState(state, sessionId, (workspace) => ({
          ...workspace,
          rightPanel: {
            ...workspace.rightPanel,
            open: maximized ? true : workspace.rightPanel.open,
            maximized,
          },
        }))
      )
    },
    setRightPanelSize(sessionId, sizePercent) {
      setState((state) =>
        updateWorkspaceState(state, sessionId, (workspace) => ({
          ...workspace,
          rightPanel: { ...workspace.rightPanel, sizePercent },
        }))
      )
    },
    setFilesSelectedRepo(sessionId, repoId) {
      setState((state) =>
        updateWorkspaceState(state, sessionId, (workspace) => ({
          ...workspace,
          files: { ...workspace.files, selectedRepoId: repoId },
        }))
      )
    },
    setFilesSelectedFile(sessionId, file) {
      setState((state) =>
        updateWorkspaceState(state, sessionId, (workspace) => ({
          ...workspace,
          files: { ...workspace.files, selectedFile: file },
        }))
      )
    },
    setFilesRepoTreeCaches(sessionId, update) {
      setState((state) =>
        updateWorkspaceState(state, sessionId, (workspace) => ({
          ...workspace,
          files: {
            ...workspace.files,
            repoTreeCaches: update(workspace.files.repoTreeCaches),
          },
        }))
      )
    },
    setBrowserURL(sessionId, url) {
      setState((state) =>
        updateWorkspaceState(state, sessionId, (workspace) => ({
          ...workspace,
          browser: { ...workspace.browser, url },
        }))
      )
    },
    navigateBrowser(sessionId, src) {
      setState((state) =>
        updateWorkspaceState(state, sessionId, (workspace) => ({
          ...workspace,
          browser: {
            ...workspace.browser,
            src,
            reloadKey: workspace.browser.reloadKey + 1,
          },
        }))
      )
    },
    openBrowserURL(sessionId, url, src) {
      setState((state) =>
        updateWorkspaceState(state, sessionId, (workspace) => ({
          ...workspace,
          rightPanel: {
            ...workspace.rightPanel,
            open: true,
            openViews: workspace.rightPanel.openViews.includes("browser")
              ? workspace.rightPanel.openViews
              : [...workspace.rightPanel.openViews, "browser"],
            activeView: "browser",
          },
          browser: {
            ...workspace.browser,
            url,
            // src defaults to the address-bar url; callers pass a distinct src
            // to show a friendly url while the iframe loads something else
            // (e.g. an app that must first go through the launch handshake).
            src: src ?? url,
            reloadKey: workspace.browser.reloadKey + 1,
          },
        }))
      )
    },
    reloadBrowser(sessionId) {
      setState((state) =>
        updateWorkspaceState(state, sessionId, (workspace) => ({
          ...workspace,
          browser: {
            ...workspace.browser,
            reloadKey: workspace.browser.reloadKey + 1,
          },
        }))
      )
    },
    openSubagentRun(sessionId, jobId) {
      setState((state) =>
        updateWorkspaceState(state, sessionId, (workspace) => ({
          ...workspace,
          rightPanel: {
            ...workspace.rightPanel,
            open: true,
            openViews: workspace.rightPanel.openViews.includes("side-chat")
              ? workspace.rightPanel.openViews
              : [...workspace.rightPanel.openViews, "side-chat"],
            activeView: "side-chat",
          },
          subagents: {
            ...workspace.subagents,
            activeJobId: jobId,
          },
        }))
      )
    },
    setReviewDiffStyle(sessionId, diffStyle) {
      setState((state) =>
        updateWorkspaceState(state, sessionId, (workspace) => ({
          ...workspace,
          review: { ...workspace.review, diffStyle },
        }))
      )
    },
  }))
)

export function useSessionWorkspaceHydration(scope: WorkspaceScope) {
  const setScope = useSessionWorkspaceStore((state) => state.setScope)
  const orgId = scope.orgId
  const userId = scope.userId
  useEffect(() => {
    setScope({ orgId, userId })
  }, [orgId, setScope, userId])
}

export function selectSessionWorkspace(
  state: SessionWorkspaceStoreState,
  sessionId?: string
) {
  if (!sessionId) return DEFAULT_SESSION_WORKSPACE
  return state.workspaces[sessionId] ?? DEFAULT_SESSION_WORKSPACE
}

export function createDraftBlobKey(sessionId: string, uploadId: string) {
  const scopeKey = useSessionWorkspaceStore.getState().scopeKey
  return `${scopeKey}:${sessionId}:${uploadId}`
}

export async function clearPersistedSessionWorkspaces() {
  if (persistTimer) {
    clearTimeout(persistTimer)
    persistTimer = null
  }
  useSessionWorkspaceStore.setState({
    status: "idle",
    workspaces: EMPTY_WORKSPACES,
  })
  await clearWorkspacePersistence()
}

function updateWorkspaceState(
  state: SessionWorkspaceStoreState,
  sessionId: string,
  update: (workspace: SessionWorkspace) => SessionWorkspace
): Pick<SessionWorkspaceStoreState, "workspaces"> {
  const current = state.workspaces[sessionId] ?? createDefaultSessionWorkspace()
  const next = {
    ...update(current),
    lastTouchedAt: Date.now(),
  }
  return {
    workspaces: {
      ...state.workspaces,
      [sessionId]: next,
    },
  }
}

useSessionWorkspaceStore.subscribe(
  (state) => [state.scopeKey, state.status, state.workspaces] as const,
  ([scopeKey, status, workspaces]) => {
    if (status !== "ready") return
    if (persistTimer) clearTimeout(persistTimer)
    persistTimer = setTimeout(() => {
      void persistWorkspaceState(scopeKey, workspaces).catch(() => undefined)
      void pruneStoredBlobs(workspaces).catch(() => undefined)
    }, PERSIST_DEBOUNCE_MS)
  }
)
