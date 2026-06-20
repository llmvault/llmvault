import type { GitStatusEntry } from "@pierre/trees"
import {
  RuntimeRepoAccessError,
  RuntimeRepoHTTPError,
  fetchRuntimeRepoFileContent,
  type RuntimeRepoTreeSnapshot,
  type RuntimeSandboxAccess,
} from "@/app/w/(chat)/_lib/runtime-repos"
import type { WorkspaceRepoTreeCache } from "@/app/w/(chat)/_stores/session-workspace-store"

const FILE_PREVIEW_LARGE_FILE_LINE_LIMIT = 2000

export function emptyRepoTreeCache(): WorkspaceRepoTreeCache {
  return {
    directoryPaths: [],
    failedDirectories: {},
    gitStatus: [],
    loadedDirectories: [],
    loadingDirectories: [],
    paths: [],
  }
}

export async function fetchPreviewFileContent(
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

export function mergeRepoTreeCache(
  cache: WorkspaceRepoTreeCache,
  loadedDirectoryPath: string,
  snapshot: RuntimeRepoTreeSnapshot
): WorkspaceRepoTreeCache {
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

export function mergeTreeSnapshots(
  root: RuntimeRepoTreeSnapshot | undefined,
  extra: Pick<WorkspaceRepoTreeCache, "directoryPaths" | "gitStatus" | "paths">
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

export function omitKey<TValue>(record: Record<string, TValue>, key: string) {
  const { [key]: _removed, ...rest } = record
  return rest
}

export function isUnauthorizedRuntimeError(error: unknown) {
  return error instanceof RuntimeRepoHTTPError && error.status === 401
}

export function errorMessage(error: unknown, fallback: string) {
  if (error instanceof RuntimeRepoAccessError) return error.message
  if (error instanceof Error && error.message.trim()) return error.message
  return fallback
}

export function formatNumber(value: number) {
  return new Intl.NumberFormat().format(value)
}

function mergeGitStatus(left: GitStatusEntry[], right: GitStatusEntry[]) {
  const merged = new Map<string, GitStatusEntry["status"]>()
  for (const entry of left) merged.set(entry.path, entry.status)
  for (const entry of right) merged.set(entry.path, entry.status)
  return [...merged.entries()].map(([path, status]) => ({ path, status }))
}

export function uniqueSorted(paths: string[]) {
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

function isLargeFileRuntimeError(error: unknown) {
  return (
    error instanceof RuntimeRepoHTTPError &&
    error.status === 400 &&
    error.message.includes("file too large")
  )
}
