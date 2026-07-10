import { createStore, clear, del, get, keys, set } from "idb-keyval"
import type {
  SessionWorkspace,
  WorkspaceRepoTreeCache,
  WorkspaceUploadItem,
} from "./session-workspace-store"

interface PersistedSessionWorkspaces {
  version: 1
  savedAt: number
  workspaces: Record<string, PersistedSessionWorkspace>
}

type PersistedSessionWorkspace = Omit<
  SessionWorkspace,
  "composer" | "files"
> & {
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

const SESSION_WORKSPACE_VERSION = 1
const SESSION_WORKSPACE_TTL_MS = 30 * 24 * 60 * 60 * 1000
const SESSION_WORKSPACE_MAX_ENTRIES = 50
const DEFAULT_RIGHT_PANEL_SIZE = 42
const WORKSPACE_STORE = createStore("hivy-session-workspaces", "ui-v1")
const WORKSPACE_BLOB_STORE = createStore(
  "hivy-session-workspace-blobs",
  "blobs-v1"
)

export const PERSIST_DEBOUNCE_MS = 350
export const EMPTY_WORKSPACES: Record<string, SessionWorkspace> = {}
export const DEFAULT_SESSION_WORKSPACE = createDefaultSessionWorkspace(0)

export function createDefaultSessionWorkspace(
  lastTouchedAt = Date.now()
): SessionWorkspace {
  return {
    lastTouchedAt,
    composer: {
      text: "",
      effort: "Low",
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
      url: "",
      src: "/",
      reloadKey: 0,
    },
    review: {
      diffStyle: "unified",
    },
    terminal: {},
    subagents: {
      activeJobId: null,
    },
    scroll: {},
  }
}

export function workspaceScopeKey(scope: {
  orgId?: string | null
  userId?: string | null
}) {
  return `${scope.orgId?.trim() || "org:unknown"}:${scope.userId?.trim() || "user:unknown"}`
}

export async function loadPersistedWorkspaces(scopeKey: string) {
  const persisted = await get<PersistedSessionWorkspaces>(
    storageKey(scopeKey),
    WORKSPACE_STORE
  )
  if (!persisted || persisted.version !== SESSION_WORKSPACE_VERSION) {
    return EMPTY_WORKSPACES
  }
  return restoreWorkspaces(persisted.workspaces)
}

export async function persistWorkspaceState(
  scopeKey: string,
  workspaces: Record<string, SessionWorkspace>
) {
  await set(
    storageKey(scopeKey),
    {
      version: SESSION_WORKSPACE_VERSION,
      savedAt: Date.now(),
      workspaces: persistableWorkspaces(workspaces),
    } satisfies PersistedSessionWorkspaces,
    WORKSPACE_STORE
  )
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

export async function clearWorkspacePersistence() {
  await Promise.allSettled([
    clear(WORKSPACE_STORE),
    clear(WORKSPACE_BLOB_STORE),
  ])
}

export async function pruneStoredBlobs(
  workspaces: Record<string, SessionWorkspace>
) {
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

function storageKey(scopeKey: string) {
  return `session-workspaces:${scopeKey}`
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
    Object.entries(pruneWorkspaces(workspaces)).map(
      ([sessionId, workspace]) => [
        sessionId,
        {
          ...workspace,
          composer: {
            ...workspace.composer,
            uploads: workspace.composer.uploads.map((upload) => {
              const {
                file: _file,
                previewUrl: _previewUrl,
                ...persisted
              } = upload
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
      ]
    )
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

async function bestEffortIDB(operation: () => Promise<unknown>) {
  try {
    await operation()
  } catch {
    // Draft attachment blob persistence is opportunistic. Metadata still
    // persists, and reads surface missing blobs back to the composer.
  }
}
