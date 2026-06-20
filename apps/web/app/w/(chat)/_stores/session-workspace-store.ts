"use client"

import { useEffect } from "react"
import { createStore, clear, del, get, keys, set } from "idb-keyval"
import { create } from "zustand"
import { subscribeWithSelector } from "zustand/middleware"
import type { GitStatusEntry } from "@pierre/trees"
import type { UploadedDriveAsset } from "@/app/w/(chat)/_lib/image-attachments"
import type {
  AttachmentDescriptionState,
} from "@/app/w/(chat)/_components/composer-attachments"
import type { CodeLineComment } from "@/app/w/(chat)/_components/line-comments"

export type SessionWorkspaceStatus = "idle" | "hydrating" | "ready"

export type WorkspacePanelViewID =
  | "review"
  | "terminal"
  | "browser"
  | "files"
  | "side-chat"

export type WorkspaceUploadStatus = "uploading" | "uploaded" | "error"

export interface WorkspaceUploadItem {
  id: string
  fileName: string
  contentType: string
  bytes: number
  lastModified: number
  blobKey?: string
  file?: File
  previewUrl?: string
  status: WorkspaceUploadStatus
  asset?: UploadedDriveAsset
  error?: string
}

export interface WorkspaceRepoTreeCache {
  directoryPaths: string[]
  failedDirectories: Record<string, string>
  gitStatus: GitStatusEntry[]
  loadedDirectories: string[]
  loadingDirectories: string[]
  paths: string[]
}

export interface SessionWorkspace {
  lastTouchedAt: number
  composer: {
    text: string
    accessMode: string
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
  scroll: {
    anchor?: string
    top?: number
  }
}

interface PersistedSessionWorkspaces {
  version: 1
  savedAt: number
  workspaces: Record<string, PersistedSessionWorkspace>
}

type PersistedSessionWorkspace = Omit<SessionWorkspace, "composer" | "files"> & {
  composer: Omit<SessionWorkspace["composer"], "uploads"> & {
    uploads: PersistedWorkspaceUploadItem[]
  }
  files: Omit<SessionWorkspace["files"], "repoTreeCaches"> & {
    repoTreeCaches: Record<string, PersistedWorkspaceRepoTreeCache>
  }
}

type PersistedWorkspaceUploadItem = Omit<
  WorkspaceUploadItem,
  "file" | "previewUrl"
>

type PersistedWorkspaceRepoTreeCache = Omit<
  WorkspaceRepoTreeCache,
  "loadingDirectories"
> & {
  loadingDirectories?: string[]
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
  setComposerAccessMode: (sessionId: string, accessMode: string) => void
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
  reloadBrowser: (sessionId: string) => void
  setReviewDiffStyle: (
    sessionId: string,
    diffStyle: SessionWorkspace["review"]["diffStyle"]
  ) => void
}

const SESSION_WORKSPACE_VERSION = 1
const SESSION_WORKSPACE_TTL_MS = 30 * 24 * 60 * 60 * 1000
const SESSION_WORKSPACE_MAX_ENTRIES = 50
const PERSIST_DEBOUNCE_MS = 350
const DEFAULT_RIGHT_PANEL_SIZE = 42
const WORKSPACE_STORE = createStore("hivy-session-workspaces", "ui-v1")
const WORKSPACE_BLOB_STORE = createStore(
  "hivy-session-workspace-blobs",
  "blobs-v1"
)
const EMPTY_WORKSPACES: Record<string, SessionWorkspace> = {}
const DEFAULT_SESSION_WORKSPACE = createDefaultSessionWorkspace(0)
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
      setState((state) => updateWorkspaceState(state, sessionId, (workspace) => workspace))
    },
    setComposerText(sessionId, text) {
      setState((state) =>
        updateWorkspaceState(state, sessionId, (workspace) => ({
          ...workspace,
          composer: { ...workspace.composer, text },
        }))
      )
    },
    setComposerAccessMode(sessionId, accessMode) {
      setState((state) =>
        updateWorkspaceState(state, sessionId, (workspace) => ({
          ...workspace,
          composer: { ...workspace.composer, accessMode },
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

export function sessionWorkspaceSnapshot(sessionId?: string) {
  if (!sessionId) return DEFAULT_SESSION_WORKSPACE
  return (
    useSessionWorkspaceStore.getState().workspaces[sessionId] ??
    DEFAULT_SESSION_WORKSPACE
  )
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

export async function storeDraftAttachmentBlob(key: string, file: File) {
  await bestEffortIDB(() => set(key, file, WORKSPACE_BLOB_STORE))
}

export async function readDraftAttachmentBlob(key: string) {
  return get<File | Blob>(key, WORKSPACE_BLOB_STORE)
}

export async function deleteDraftAttachmentBlob(key?: string) {
  if (!key) return
  await bestEffortIDB(() => del(key, WORKSPACE_BLOB_STORE))
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
  await Promise.allSettled([clear(WORKSPACE_STORE), clear(WORKSPACE_BLOB_STORE)])
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

function createDefaultSessionWorkspace(lastTouchedAt = Date.now()): SessionWorkspace {
  return {
    lastTouchedAt,
    composer: {
      text: "",
      accessMode: "full",
      effort: "High",
      uploads: [],
      attachmentDescriptions: {},
    },
    lineComments: [],
    rightPanel: {
      open: false,
      openViews: [],
      activeView: null,
      maximized: false,
      sizePercent: DEFAULT_RIGHT_PANEL_SIZE,
    },
    files: {
      selectedRepoId: null,
      selectedFile: null,
      repoTreeCaches: {},
    },
    browser: {
      url: "usehivy.com",
      src: "/",
      reloadKey: 0,
    },
    review: {
      diffStyle: "unified",
    },
    terminal: {},
    scroll: {},
  }
}

function workspaceScopeKey(scope: WorkspaceScope) {
  return `${scope.orgId?.trim() || "org:unknown"}:${scope.userId?.trim() || "user:unknown"}`
}

function storageKey(scopeKey: string) {
  return `session-workspaces:${scopeKey}`
}

async function loadPersistedWorkspaces(scopeKey: string) {
  const persisted = await get<PersistedSessionWorkspaces>(
    storageKey(scopeKey),
    WORKSPACE_STORE
  )
  if (!persisted || persisted.version !== SESSION_WORKSPACE_VERSION) {
    return EMPTY_WORKSPACES
  }
  return restoreWorkspaces(persisted.workspaces)
}

function restoreWorkspaces(
  persisted: Record<string, PersistedSessionWorkspace>
) {
  const now = Date.now()
  const restored: Record<string, SessionWorkspace> = {}
  for (const [sessionId, workspace] of Object.entries(persisted)) {
    if (now - workspace.lastTouchedAt > SESSION_WORKSPACE_TTL_MS) continue
    const defaults = createDefaultSessionWorkspace(0)
    restored[sessionId] = {
      ...defaults,
      ...workspace,
      rightPanel: {
        ...defaults.rightPanel,
        ...workspace.rightPanel,
        open:
          workspace.rightPanel.open ??
          (workspace.rightPanel.openViews?.length ?? 0) > 0,
      },
      composer: {
        ...defaults.composer,
        ...workspace.composer,
        uploads: workspace.composer.uploads.map((upload) => ({
          ...upload,
          previewUrl: upload.asset?.asset_url,
        })),
      },
      files: {
        ...defaults.files,
        ...workspace.files,
        repoTreeCaches: Object.fromEntries(
          Object.entries(workspace.files.repoTreeCaches ?? {}).map(
            ([repoId, cache]) => [
              repoId,
              {
                ...cache,
                loadingDirectories: [],
              } satisfies WorkspaceRepoTreeCache,
            ]
          )
        ),
      },
    }
  }
  return pruneWorkspaces(restored)
}

function persistableWorkspaces(
  workspaces: Record<string, SessionWorkspace>
): Record<string, PersistedSessionWorkspace> {
  return Object.fromEntries(
    Object.entries(pruneWorkspaces(workspaces)).map(([sessionId, workspace]) => [
      sessionId,
      {
        ...workspace,
        composer: {
          ...workspace.composer,
          uploads: workspace.composer.uploads.map((upload) => {
            const { file: _file, previewUrl: _previewUrl, ...persisted } = upload
            return persisted
          }),
        },
        files: {
          ...workspace.files,
          repoTreeCaches: Object.fromEntries(
            Object.entries(workspace.files.repoTreeCaches).map(
              ([repoId, cache]) => [
                repoId,
                {
                  ...cache,
                  loadingDirectories: [],
                },
              ]
            )
          ),
        },
      },
    ])
  )
}

function pruneWorkspaces(workspaces: Record<string, SessionWorkspace>) {
  const now = Date.now()
  return Object.fromEntries(
    Object.entries(workspaces)
      .filter(
        ([, workspace]) =>
          now - workspace.lastTouchedAt <= SESSION_WORKSPACE_TTL_MS
      )
      .sort((left, right) => right[1].lastTouchedAt - left[1].lastTouchedAt)
      .slice(0, SESSION_WORKSPACE_MAX_ENTRIES)
  )
}

useSessionWorkspaceStore.subscribe(
  (state) => [state.scopeKey, state.status, state.workspaces] as const,
  ([scopeKey, status, workspaces]) => {
    if (status !== "ready") return
    if (persistTimer) clearTimeout(persistTimer)
    persistTimer = setTimeout(() => {
      void set(
        storageKey(scopeKey),
        {
          version: SESSION_WORKSPACE_VERSION,
          savedAt: Date.now(),
          workspaces: persistableWorkspaces(workspaces),
        } satisfies PersistedSessionWorkspaces,
        WORKSPACE_STORE
      ).catch(() => undefined)
      void pruneStoredBlobs(workspaces).catch(() => undefined)
    }, PERSIST_DEBOUNCE_MS)
  }
)

async function pruneStoredBlobs(workspaces: Record<string, SessionWorkspace>) {
  const blobKeys = new Set(
    Object.values(workspaces).flatMap((workspace) =>
      workspace.composer.uploads.flatMap((upload) =>
        upload.blobKey ? [upload.blobKey] : []
      )
    )
  )
  const storedKeys = await keys(WORKSPACE_BLOB_STORE)
  await Promise.all(
    storedKeys.map((key) =>
      typeof key === "string" && !blobKeys.has(key)
        ? del(key, WORKSPACE_BLOB_STORE)
        : Promise.resolve()
    )
  )
}

async function bestEffortIDB(operation: () => Promise<unknown>) {
  try {
    await operation()
  } catch {
    // Draft attachment blob persistence is opportunistic. Metadata still
    // persists, and reads surface missing blobs back to the composer.
  }
}
