"use client"

import {
  type CSSProperties,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from "react"
import type { GitStatusEntry } from "@pierre/trees"
import { Button } from "@heroui/react"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Icon } from "@iconify/react"
import { Group, Panel, Separator } from "react-resizable-panels"
import {
  CommentableFile,
  type CommentableFileProps,
} from "@/app/w/(chat)/_components/diff-line-comments"
import {
  FilesEmptyState,
  FilesErrorState,
  FilesLoadingState,
  RuntimeFileTree,
  TreeSkeleton,
} from "@/app/w/(chat)/_components/views/files-ui"
import {
  RuntimeRepoAccessError,
  RuntimeRepoHTTPError,
  fetchRuntimeRepoFileContent,
  fetchRuntimeRepoTreeDirectory,
  fetchRuntimeRepos,
  type RuntimeRepoContent,
  type RuntimeRepoInfo,
  type RuntimeRepoTreeSnapshot,
  type RuntimeSandboxAccess,
} from "@/app/w/(chat)/_lib/runtime-repos"
import { HIVY_DIFF_STYLE, hivyDiffOptions } from "@/lib/diffs-theme"
import {
  FilesRepoSelector,
  type FilesRepoSelectorProps,
} from "./files-repo-selector"

export { FilesRepoSelector }
export type { FilesRepoSelectorProps }

interface FilesViewProps {
  sessionId?: string
  sandboxAccess?: RuntimeSandboxAccess
  sandboxAccessPending: boolean
  sandboxAccessError: unknown
  onRefreshSandboxAccess: () => void
  onHeaderChange: (props: FilesRepoSelectorProps | null) => void
}

const FILE_TREE_DEFAULT_WIDTH = 260
const FILE_TREE_MIN_WIDTH = 190
const FILE_TREE_MAX_WIDTH = 420
const FILE_PREVIEW_MIN_WIDTH = 120
const FILE_PREVIEW_LARGE_FILE_LINE_LIMIT = 2000
const FILE_PREVIEW_OPTIONS = hivyDiffOptions({
  disableFileHeader: true,
  overflow: "scroll",
}) satisfies NonNullable<CommentableFileProps["options"]>
const FILE_PREVIEW_STYLE: CSSProperties & Record<`--${string}`, string> = {
  ...HIVY_DIFF_STYLE,
  minHeight: "100%",
  width: "100%",
}

interface RepoTreeCache {
  directoryPaths: string[]
  failedDirectories: Record<string, string>
  gitStatus: GitStatusEntry[]
  loadedDirectories: string[]
  loadingDirectories: string[]
  paths: string[]
}

export function FilesView({
  sessionId,
  sandboxAccess,
  sandboxAccessPending,
  sandboxAccessError,
  onRefreshSandboxAccess,
  onHeaderChange,
}: FilesViewProps) {
  const queryClient = useQueryClient()
  const [selectedRepoId, setSelectedRepoId] = useState<string | null>(null)
  const [selectedFile, setSelectedFile] = useState<{
    path: string
    repoId: string
  } | null>(null)
  const [repoTreeCaches, setRepoTreeCaches] = useState<
    Record<string, RepoTreeCache>
  >({})
  const accessMatchesSession = sandboxAccess?.session_id === sessionId
  const accessReady = Boolean(
    sessionId &&
    accessMatchesSession &&
    sandboxAccess?.sandbox_base_url &&
    sandboxAccess?.token
  )
  const reposQuery = useQuery({
    enabled: accessReady,
    queryKey: [
      "sandbox-runtime-repos",
      sessionId,
      sandboxAccess?.sandbox_base_url,
      sandboxAccess?.token,
    ],
    queryFn: ({ signal }) => fetchRuntimeRepos(sandboxAccess ?? {}, signal),
    retry: false,
  })
  const repos = useMemo(() => reposQuery.data ?? [], [reposQuery.data])
  const selectedRepo =
    repos.find((repo) => repo.id === selectedRepoId) ?? repos[0] ?? null
  const selectedPath =
    selectedFile && selectedRepo && selectedFile.repoId === selectedRepo.id
      ? selectedFile.path
      : null

  const rootTreeQuery = useQuery({
    enabled: accessReady && Boolean(selectedRepo?.id),
    queryKey: [
      "sandbox-runtime-repo-tree-directory",
      sessionId,
      sandboxAccess?.sandbox_base_url,
      sandboxAccess?.token,
      selectedRepo?.id,
      "",
    ],
    queryFn: ({ signal }) =>
      fetchRuntimeRepoTreeDirectory(
        sandboxAccess ?? {},
        selectedRepo?.id ?? "",
        "",
        signal
      ),
    retry: false,
  })
  const selectedRepoTreeCache = selectedRepo
    ? repoTreeCaches[selectedRepo.id]
    : undefined
  const loadedDirectoryPaths = useMemo(
    () => new Set(["", ...(selectedRepoTreeCache?.loadedDirectories ?? [])]),
    [selectedRepoTreeCache?.loadedDirectories]
  )
  const loadingDirectoryPaths = useMemo(
    () => new Set(selectedRepoTreeCache?.loadingDirectories ?? []),
    [selectedRepoTreeCache?.loadingDirectories]
  )
  const mergedTree = useMemo(
    () =>
      mergeTreeSnapshots(rootTreeQuery.data, {
        directoryPaths: selectedRepoTreeCache?.directoryPaths ?? [],
        gitStatus: selectedRepoTreeCache?.gitStatus ?? [],
        paths: selectedRepoTreeCache?.paths ?? [],
      }),
    [
      rootTreeQuery.data,
      selectedRepoTreeCache?.directoryPaths,
      selectedRepoTreeCache?.gitStatus,
      selectedRepoTreeCache?.paths,
    ]
  )
  const directoryPathSet = useMemo(
    () => new Set(mergedTree.directoryPaths),
    [mergedTree.directoryPaths]
  )
  const filePreviewQuery = useQuery({
    enabled: accessReady && Boolean(selectedRepo?.id && selectedPath),
    queryKey: [
      "sandbox-runtime-repo-content",
      sessionId,
      sandboxAccess?.sandbox_base_url,
      sandboxAccess?.token,
      selectedRepo?.id,
      selectedPath,
    ],
    queryFn: ({ signal }) =>
      fetchPreviewFileContent(
        sandboxAccess ?? {},
        selectedRepo?.id ?? "",
        selectedPath ?? "",
        signal
      ),
    retry: false,
  })
  const loadDirectory = useCallback(
    (directoryPath: string) => {
      if (!accessReady || !selectedRepo?.id) return
      if (
        loadedDirectoryPaths.has(directoryPath) ||
        loadingDirectoryPaths.has(directoryPath)
      ) {
        return
      }
      const repoId = selectedRepo.id
      setRepoTreeCaches((current) => {
        const cache = current[repoId] ?? emptyRepoTreeCache()
        return {
          ...current,
          [repoId]: {
            ...cache,
            failedDirectories: omitKey(cache.failedDirectories, directoryPath),
            loadingDirectories: uniqueSorted([
              ...cache.loadingDirectories,
              directoryPath,
            ]),
          },
        }
      })
      void queryClient
        .fetchQuery({
          queryKey: [
            "sandbox-runtime-repo-tree-directory",
            sessionId,
            sandboxAccess?.sandbox_base_url,
            sandboxAccess?.token,
            repoId,
            directoryPath,
          ],
          queryFn: ({ signal }) =>
            fetchRuntimeRepoTreeDirectory(
              sandboxAccess ?? {},
              repoId,
              directoryPath,
              signal
            ),
          retry: false,
          staleTime: 5 * 60 * 1000,
        })
        .then((snapshot) => {
          setRepoTreeCaches((current) => {
            const cache = current[repoId] ?? emptyRepoTreeCache()
            return {
              ...current,
              [repoId]: mergeRepoTreeCache(cache, directoryPath, snapshot),
            }
          })
        })
        .catch((error: unknown) => {
          setRepoTreeCaches((current) => {
            const cache = current[repoId] ?? emptyRepoTreeCache()
            return {
              ...current,
              [repoId]: {
                ...cache,
                failedDirectories: {
                  ...cache.failedDirectories,
                  [directoryPath]: errorMessage(
                    error,
                    "Could not load directory."
                  ),
                },
                loadingDirectories: cache.loadingDirectories.filter(
                  (path) => path !== directoryPath
                ),
              },
            }
          })
        })
    },
    [
      accessReady,
      loadedDirectoryPaths,
      loadingDirectoryPaths,
      queryClient,
      sandboxAccess,
      selectedRepo,
      sessionId,
    ]
  )
  useEffect(() => {
    if (repos.length === 0) {
      onHeaderChange(null)
      return
    }
    onHeaderChange({
      repos,
      selectedRepo,
      onSelect: setSelectedRepoId,
    })
    return () => onHeaderChange(null)
  }, [onHeaderChange, repos, selectedRepo])

  if (!sessionId) {
    return (
      <FilesEmptyState
        icon="lucide:folder-x"
        title="No active session"
        message="Open a session to browse sandbox files."
      />
    )
  }

  if (sandboxAccessPending && !accessReady) {
    return <FilesLoadingState label="Connecting to sandbox" />
  }

  if (sandboxAccessError && !accessReady) {
    return (
      <FilesErrorState
        message={errorMessage(
          sandboxAccessError,
          "Sandbox access is not available."
        )}
        onRetry={onRefreshSandboxAccess}
      />
    )
  }

  if (reposQuery.isPending) {
    return <FilesLoadingState label="Loading repositories" />
  }

  if (reposQuery.isError) {
    return (
      <FilesErrorState
        message={errorMessage(reposQuery.error, "Could not load repositories.")}
        onRetry={() => {
          if (isUnauthorizedRuntimeError(reposQuery.error)) {
            onRefreshSandboxAccess()
          }
          void reposQuery.refetch()
        }}
      />
    )
  }

  if (repos.length === 0) {
    return (
      <FilesEmptyState
        icon="lucide:folder-search"
        title="No repositories found"
        message="The sandbox has no Git repositories under its workspace repos directory."
      />
    )
  }

  return (
    <div className="bg-surface flex h-full min-w-0 flex-col">
      <Group
        id="files-view-layout"
        orientation="horizontal"
        className="min-h-0 flex-1"
      >
        <Panel
          id="files-preview"
          minSize={FILE_PREVIEW_MIN_WIDTH}
          className="min-w-0 overflow-hidden"
        >
          <section className="flex h-full min-w-0 flex-col bg-background">
            <div className="flex h-10 shrink-0 items-center border-b border-border px-3 text-sm text-muted">
              {selectedPath ? (
                <span className="truncate">
                  {selectedPath.replace(/\/$/, "")}
                </span>
              ) : (
                <span>Select a file to preview</span>
              )}
            </div>
            <div className="min-h-0 flex-1 overflow-hidden">
              <FilePreview
                content={filePreviewQuery.data}
                error={filePreviewQuery.error}
                isPending={filePreviewQuery.isPending}
                path={selectedPath}
                repo={selectedRepo}
                onRefreshSandboxAccess={onRefreshSandboxAccess}
                onRetry={() => void filePreviewQuery.refetch()}
              />
            </div>
          </section>
        </Panel>

        <Separator
          id="files-tree-resize-handle"
          className="w-px shrink-0 bg-border transition-colors hover:bg-accent data-[resizing]:bg-accent"
        />

        <Panel
          id="files-tree"
          defaultSize={FILE_TREE_DEFAULT_WIDTH}
          minSize={FILE_TREE_MIN_WIDTH}
          maxSize={FILE_TREE_MAX_WIDTH}
          groupResizeBehavior="preserve-pixel-size"
          className="min-w-0 overflow-hidden"
        >
          <aside className="bg-surface h-full min-w-0">
            {rootTreeQuery.isPending ? (
              <TreeSkeleton />
            ) : rootTreeQuery.isError ? (
              <div className="flex h-full flex-col items-center justify-center gap-3 px-4 text-center">
                <Icon
                  icon="lucide:circle-alert"
                  className="h-5 w-5 text-muted"
                />
                <p className="text-sm text-muted">
                  {errorMessage(
                    rootTreeQuery.error,
                    "Could not load file tree."
                  )}
                </p>
                <Button
                  size="sm"
                  variant="secondary"
                  onPress={() => {
                    if (isUnauthorizedRuntimeError(rootTreeQuery.error)) {
                      onRefreshSandboxAccess()
                    }
                    void rootTreeQuery.refetch()
                  }}
                >
                  Retry
                </Button>
              </div>
            ) : (
              <RuntimeFileTree
                directoryPaths={mergedTree.directoryPaths}
                gitStatus={mergedTree.gitStatus}
                loadedDirectoryPaths={loadedDirectoryPaths}
                loadingDirectoryPaths={loadingDirectoryPaths}
                paths={mergedTree.paths}
                selectedPath={selectedPath}
                onDirectoryExpand={loadDirectory}
                onSelectPath={(path) => {
                  if (!path || !selectedRepo || directoryPathSet.has(path)) {
                    setSelectedFile(null)
                    return
                  }
                  setSelectedFile({
                    path,
                    repoId: selectedRepo.id,
                  })
                }}
              />
            )}
          </aside>
        </Panel>
      </Group>
    </div>
  )
}

function FilePreview({
  path,
  repo,
  content,
  isPending,
  error,
  onRefreshSandboxAccess,
  onRetry,
}: {
  path: string | null
  repo: RuntimeRepoInfo | null
  content?: RuntimeRepoContent
  isPending: boolean
  error: unknown
  onRefreshSandboxAccess: () => void
  onRetry: () => void
}) {
  if (!path) {
    return (
      <div className="flex h-full items-center justify-center px-6 text-center text-sm text-muted">
        Select a file to preview.
      </div>
    )
  }

  if (isPending) {
    return <FilesLoadingState label="Loading file" />
  }

  if (error) {
    return (
      <FilesErrorState
        message={errorMessage(error, "Could not load file.")}
        onRetry={() => {
          if (isUnauthorizedRuntimeError(error)) {
            onRefreshSandboxAccess()
          }
          onRetry()
        }}
      />
    )
  }

  if (!content) {
    return null
  }

  if (content.content.length === 0) {
    return (
      <div className="flex h-full items-center justify-center px-6 text-center text-sm text-muted">
        File is empty.
      </div>
    )
  }

  const cacheKey = [
    content.repo_id,
    content.path,
    content.total_bytes,
    content.shown_lines,
    content.offset ?? 0,
    content.limit ?? 0,
  ].join(":")

  return (
    <div className="flex h-full min-h-0 flex-col bg-background">
      {content.truncated ? (
        <div className="shrink-0 border-b border-border px-3 py-2 text-xs text-muted">
          Showing {formatNumber(content.shown_lines)} of{" "}
          {formatNumber(content.total_lines)} lines.
        </div>
      ) : null}
      <div className="min-h-0 flex-1 overflow-x-hidden overflow-y-auto">
        <CommentableFile
          key={cacheKey}
          file={{
            name: content.path || path,
            contents: content.content,
            cacheKey,
          }}
          options={FILE_PREVIEW_OPTIONS}
          source={{
            kind: "file",
            path: content.path || path || undefined,
            repoId: content.repo_id,
            repoName: repo?.name,
            repoPath: repo?.relative_path,
          }}
          className="block min-w-full"
          style={FILE_PREVIEW_STYLE}
          disableWorkerPool
        />
      </div>
    </div>
  )
}

function emptyRepoTreeCache(): RepoTreeCache {
  return {
    directoryPaths: [],
    failedDirectories: {},
    gitStatus: [],
    loadedDirectories: [],
    loadingDirectories: [],
    paths: [],
  }
}

async function fetchPreviewFileContent(
  access: RuntimeSandboxAccess,
  repoId: string,
  path: string,
  signal?: AbortSignal
) {
  try {
    return await fetchRuntimeRepoFileContent(access, repoId, path, signal)
  } catch (error) {
    if (isLargeFileRuntimeError(error)) {
      return fetchRuntimeRepoFileContent(access, repoId, path, signal, {
        offset: 1,
        limit: FILE_PREVIEW_LARGE_FILE_LINE_LIMIT,
      })
    }
    throw error
  }
}

function mergeRepoTreeCache(
  cache: RepoTreeCache,
  loadedDirectoryPath: string,
  snapshot: RuntimeRepoTreeSnapshot
): RepoTreeCache {
  return {
    directoryPaths: uniqueSorted([
      ...cache.directoryPaths,
      ...snapshot.directoryPaths,
    ]),
    failedDirectories: omitKey(cache.failedDirectories, loadedDirectoryPath),
    gitStatus: mergeGitStatus(cache.gitStatus, snapshot.gitStatus),
    loadedDirectories: uniqueSorted([
      ...cache.loadedDirectories,
      loadedDirectoryPath,
    ]),
    loadingDirectories: cache.loadingDirectories.filter(
      (path) => path !== loadedDirectoryPath
    ),
    paths: uniqueSorted([...cache.paths, ...snapshot.paths]),
  }
}

function mergeTreeSnapshots(
  root: RuntimeRepoTreeSnapshot | undefined,
  extra: Pick<RepoTreeCache, "directoryPaths" | "gitStatus" | "paths">
): RuntimeRepoTreeSnapshot {
  return {
    directoryPaths: uniqueSorted([
      ...(root?.directoryPaths ?? []),
      ...extra.directoryPaths,
    ]),
    gitStatus: mergeGitStatus(root?.gitStatus ?? [], extra.gitStatus),
    paths: uniqueSorted([...(root?.paths ?? []), ...extra.paths]),
  }
}

function mergeGitStatus(left: GitStatusEntry[], right: GitStatusEntry[]) {
  const merged = new Map<string, GitStatusEntry["status"]>()
  for (const entry of left) merged.set(entry.path, entry.status)
  for (const entry of right) merged.set(entry.path, entry.status)
  return [...merged.entries()].map(([path, status]) => ({ path, status }))
}

function uniqueSorted(paths: string[]) {
  return [...new Set(paths)].sort(compareFileTreePaths)
}

function compareFileTreePaths(left: string, right: string) {
  const leftSegments = left.split("/")
  const rightSegments = right.split("/")
  const length = Math.min(leftSegments.length, rightSegments.length)
  for (let index = 0; index < length; index += 1) {
    const leftPart = leftSegments[index] ?? ""
    const rightPart = rightSegments[index] ?? ""
    if (leftPart === rightPart) continue
    return leftPart.localeCompare(rightPart)
  }
  return leftSegments.length - rightSegments.length
}

function omitKey<TValue>(record: Record<string, TValue>, key: string) {
  const { [key]: _removed, ...rest } = record
  return rest
}

function isUnauthorizedRuntimeError(error: unknown) {
  return error instanceof RuntimeRepoHTTPError && error.status === 401
}

function isLargeFileRuntimeError(error: unknown) {
  return (
    error instanceof RuntimeRepoHTTPError &&
    error.status === 400 &&
    error.message.includes("file too large")
  )
}

function errorMessage(error: unknown, fallback: string) {
  if (error instanceof RuntimeRepoAccessError) return error.message
  if (error instanceof Error && error.message.trim()) return error.message
  return fallback
}

function formatNumber(value: number) {
  return new Intl.NumberFormat().format(value)
}
