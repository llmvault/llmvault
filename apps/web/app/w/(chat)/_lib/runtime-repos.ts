import type { GitStatusEntry } from "@pierre/trees"

export interface RuntimeSandboxAccess {
  session_id?: string
  sandbox_base_url?: string
  token?: string
}

export interface RuntimeRepoInfo {
  id: string
  name: string
  relative_path: string
  head_sha: string
  base_sha: string
}

interface RuntimeRepoListResponse {
  repos: RuntimeRepoInfo[]
}

export interface RuntimeTreeEntry {
  name: string
  path: string
  type: "directory" | "file"
  size?: number | null
  git_status?: RuntimeGitStatus | null
}

interface RuntimeTreeResponse {
  repo_id: string
  path: string
  entries: RuntimeTreeEntry[]
}

export interface RuntimeRepoContent {
  repo_id: string
  path: string
  content: string
  encoding: string
  truncated: boolean
  total_lines: number
  total_bytes: number
  shown_lines: number
  offset?: number | null
  limit?: number | null
}

export interface RuntimeRepoDiff {
  repo_id: string
  path?: string | null
  diff: string
  truncated?: boolean
  total_bytes?: number
  max_bytes?: number
  files?: RuntimeRepoDiffFile[]
  message?: string | null
}

export interface RuntimeRepoDiffFile {
  path: string
  status: RuntimeGitStatus | "copied" | "unknown"
  previous_path?: string | null
  patch: string
  truncated: boolean
  total_bytes: number
  max_bytes: number
  message?: string | null
}

export type RuntimeGitStatus =
  | "added"
  | "deleted"
  | "ignored"
  | "modified"
  | "renamed"
  | "untracked"

export interface RuntimeRepoTreeSnapshot {
  directoryPaths: string[]
  gitStatus: GitStatusEntry[]
  paths: string[]
}

export async function fetchRuntimeRepos(
  access: RuntimeSandboxAccess,
  signal?: AbortSignal
) {
  const response = await runtimeJSON<RuntimeRepoListResponse>(
    access,
    "/repos",
    signal
  )
  return response.repos
}

export async function fetchRuntimeRepoTreeDirectory(
  access: RuntimeSandboxAccess,
  repoId: string,
  path: string,
  signal?: AbortSignal
): Promise<RuntimeRepoTreeSnapshot> {
  const tree = await fetchRuntimeRepoTree(access, repoId, path, signal)
  return treeEntriesToSnapshot(tree.entries)
}

export async function fetchRuntimeRepoFileContent(
  access: RuntimeSandboxAccess,
  repoId: string,
  path: string,
  signal?: AbortSignal,
  options: { offset?: number; limit?: number } = {}
): Promise<RuntimeRepoContent> {
  const search = new URLSearchParams()
  search.set("path", path)
  if (typeof options.offset === "number") {
    search.set("offset", String(options.offset))
  }
  if (typeof options.limit === "number") {
    search.set("limit", String(options.limit))
  }
  return runtimeJSON<RuntimeRepoContent>(
    access,
    `/repos/${encodeURIComponent(repoId)}/content?${search.toString()}`,
    signal
  )
}

export async function fetchRuntimeRepoDiff(
  access: RuntimeSandboxAccess,
  repoId: string,
  signal?: AbortSignal,
  options: { path?: string; context?: number } = {}
): Promise<RuntimeRepoDiff> {
  const search = new URLSearchParams()
  if (options.path) {
    search.set("path", options.path)
  }
  if (typeof options.context === "number") {
    search.set("context", String(options.context))
  }
  const query = search.toString()
  return runtimeJSON<RuntimeRepoDiff>(
    access,
    `/repos/${encodeURIComponent(repoId)}/diff${query ? `?${query}` : ""}`,
    signal
  )
}

async function fetchRuntimeRepoTree(
  access: RuntimeSandboxAccess,
  repoId: string,
  path: string,
  signal?: AbortSignal
) {
  const search = new URLSearchParams()
  search.set("path", path)
  return runtimeJSON<RuntimeTreeResponse>(
    access,
    `/repos/${encodeURIComponent(repoId)}/tree?${search.toString()}`,
    signal
  )
}

function treeEntriesToSnapshot(entries: RuntimeTreeEntry[]) {
  const paths = new Set<string>()
  const directoryPaths = new Set<string>()
  const gitStatus = new Map<string, RuntimeGitStatus>()
  for (const entry of entries) {
    if (entry.type === "directory") {
      const directoryPath = entry.path.replace(/\/+$/, "")
      directoryPaths.add(directoryPath)
      paths.add(`${directoryPath}/`)
    } else {
      paths.add(entry.path)
    }
    if (entry.git_status) {
      gitStatus.set(entry.path, entry.git_status)
    }
  }
  return {
    directoryPaths: [...directoryPaths].sort(compareFileTreePaths),
    paths: [...paths].sort(compareFileTreePaths),
    gitStatus: [...gitStatus.entries()].map(([path, status]) => ({
      path,
      status,
    })),
  }
}

async function runtimeJSON<T>(
  access: RuntimeSandboxAccess,
  path: string,
  signal?: AbortSignal
): Promise<T> {
  const baseURL = access.sandbox_base_url?.replace(/\/+$/, "")
  const token = access.token
  if (!baseURL || !token) {
    throw new RuntimeRepoAccessError("Sandbox access is not available.")
  }
  // eslint-disable-next-line no-restricted-globals -- direct-sandbox call: hits the sandbox base URL with a sandbox token, not the Hivy API.
  const response = await fetch(`${baseURL}${path}`, {
    headers: {
      Accept: "application/json",
      Authorization: `Bearer ${token}`,
    },
    signal,
  })
  if (!response.ok) {
    const body = await response.text().catch(() => "")
    const detail = body.trim() ? `: ${body.trim()}` : ""
    throw new RuntimeRepoHTTPError(
      response.status,
      `Sandbox repository request failed with HTTP ${response.status}${detail}`
    )
  }
  return (await response.json()) as T
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

export class RuntimeRepoAccessError extends Error {}

export class RuntimeRepoHTTPError extends Error {
  constructor(
    readonly status: number,
    message: string
  ) {
    super(message)
  }
}
