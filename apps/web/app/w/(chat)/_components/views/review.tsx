"use client"

import { useMemo, useState, type CSSProperties } from "react"
import { Button, Spinner } from "@heroui/react"
import { Icon } from "@iconify/react"
import { PatchDiff, type PatchDiffProps } from "@pierre/diffs/react"
import { useQuery } from "@tanstack/react-query"
import {
  RuntimeRepoAccessError,
  RuntimeRepoHTTPError,
  fetchRuntimeRepoDiff,
  fetchRuntimeRepos,
  type RuntimeRepoInfo,
  type RuntimeSandboxAccess,
} from "@/app/w/(chat)/_lib/runtime-repos"

type ReviewDiffStyle = "unified" | "split"
type ReviewDiffOptions = NonNullable<PatchDiffProps<undefined>["options"]>

interface ReviewViewProps {
  sessionId?: string
  sandboxAccess?: RuntimeSandboxAccess
  sandboxAccessPending: boolean
  sandboxAccessError: unknown
  onRefreshSandboxAccess: () => void
}

interface ReviewRepoDiff {
  repo: RuntimeRepoInfo
  patches: string[]
}

interface ReviewDiffsResult {
  repos: RuntimeRepoInfo[]
  repoDiffs: ReviewRepoDiff[]
}

const REVIEW_DIFF_BASE_OPTIONS = {
  overflow: "scroll",
  theme: "pierre-dark",
  themeType: "dark",
} satisfies ReviewDiffOptions

const REVIEW_DIFF_STYLE: CSSProperties & Record<`--${string}`, string> = {
  "--diffs-font-size": "12px",
  "--diffs-line-height": "20px",
}

const DIFF_STYLE_OPTIONS: {
  id: ReviewDiffStyle
  label: string
  icon: string
}[] = [
  { id: "unified", label: "Unified", icon: "lucide:rows-3" },
  { id: "split", label: "Split", icon: "lucide:columns-2" },
]

export function ReviewView({
  sessionId,
  sandboxAccess,
  sandboxAccessPending,
  sandboxAccessError,
  onRefreshSandboxAccess,
}: ReviewViewProps) {
  const [diffStyle, setDiffStyle] = useState<ReviewDiffStyle>("unified")
  const accessMatchesSession = sandboxAccess?.session_id === sessionId
  const accessReady = Boolean(
    sessionId &&
      accessMatchesSession &&
      sandboxAccess?.sandbox_base_url &&
      sandboxAccess?.token
  )
  const reviewQuery = useQuery({
    enabled: accessReady,
    queryKey: [
      "sandbox-runtime-review-diffs",
      sessionId,
      sandboxAccess?.sandbox_base_url,
      sandboxAccess?.token,
    ],
    queryFn: ({ signal }) => fetchReviewDiffs(sandboxAccess ?? {}, signal),
    retry: false,
  })
  const diffOptions = useMemo<ReviewDiffOptions>(
    () => ({
      ...REVIEW_DIFF_BASE_OPTIONS,
      diffStyle,
    }),
    [diffStyle]
  )
  const changedRepoDiffs = useMemo(
    () =>
      reviewQuery.data?.repoDiffs.filter(
        (repoDiff) => repoDiff.patches.length > 0
      ) ?? [],
    [reviewQuery.data?.repoDiffs]
  )
  const patchCount = changedRepoDiffs.reduce(
    (total, repoDiff) => total + repoDiff.patches.length,
    0
  )

  if (!sessionId) {
    return (
      <ReviewEmptyState
        icon="lucide:file-diff"
        title="No active session"
        message="Open a session to review sandbox changes."
      />
    )
  }

  if (sandboxAccessPending && !accessReady) {
    return <ReviewLoadingState label="Connecting to sandbox" />
  }

  if (sandboxAccessError && !accessReady) {
    return (
      <ReviewErrorState
        message={errorMessage(
          sandboxAccessError,
          "Sandbox access is not available."
        )}
        onRetry={onRefreshSandboxAccess}
      />
    )
  }

  if (reviewQuery.isPending) {
    return <ReviewLoadingState label="Loading diffs" />
  }

  if (reviewQuery.isError) {
    return (
      <ReviewErrorState
        message={errorMessage(reviewQuery.error, "Could not load diffs.")}
        onRetry={() => {
          if (isUnauthorizedRuntimeError(reviewQuery.error)) {
            onRefreshSandboxAccess()
          }
          void reviewQuery.refetch()
        }}
      />
    )
  }

  if (reviewQuery.data?.repos.length === 0) {
    return (
      <ReviewEmptyState
        icon="lucide:folder-search"
        title="No repositories found"
        message="The sandbox has no Git repositories under its workspace repos directory."
      />
    )
  }

  return (
    <div className="flex h-full min-w-0 flex-col bg-background">
      <div className="flex h-10 shrink-0 items-center gap-2 border-b border-border px-3">
        <div className="flex min-w-0 flex-1 items-center gap-2">
          <Icon icon="lucide:file-diff" className="h-4 w-4 shrink-0 text-muted" />
          <span className="truncate text-sm font-medium">Changes</span>
          <span className="shrink-0 text-xs text-muted">
            {formatPatchCount(patchCount)}
          </span>
        </div>
        <DiffStyleToggle value={diffStyle} onChange={setDiffStyle} />
        <Button
          aria-label="Refresh diffs"
          size="sm"
          variant="ghost"
          isIconOnly
          isDisabled={reviewQuery.isFetching}
          onPress={() => void reviewQuery.refetch()}
        >
          <Icon
            icon="lucide:refresh-cw"
            className={`h-4 w-4 text-muted ${
              reviewQuery.isFetching ? "animate-spin" : ""
            }`}
          />
        </Button>
      </div>

      <div className="min-h-0 flex-1 overflow-x-hidden overflow-y-auto bg-surface">
        {changedRepoDiffs.length > 0 ? (
          <div className="flex min-h-full min-w-0 flex-col gap-3">
            {changedRepoDiffs.map((repoDiff) => (
              <RepoDiffSection
                key={repoDiff.repo.id}
                repoDiff={repoDiff}
                options={diffOptions}
              />
            ))}
          </div>
        ) : (
          <ReviewEmptyState
            icon="lucide:check-circle-2"
            title="No changes to review"
            message="Sandbox repositories currently match their base commits."
          />
        )}
      </div>
    </div>
  )
}

function DiffStyleToggle({
  value,
  onChange,
}: {
  value: ReviewDiffStyle
  onChange: (value: ReviewDiffStyle) => void
}) {
  return (
    <div className="flex shrink-0 items-center gap-0.5 rounded-lg bg-surface-secondary p-0.5">
      {DIFF_STYLE_OPTIONS.map((option) => (
        <button
          key={option.id}
          type="button"
          aria-pressed={option.id === value}
          onClick={() => onChange(option.id)}
          className={`flex h-7 items-center gap-1.5 rounded-md px-2 text-xs transition-colors ${
            option.id === value
              ? "bg-background text-foreground shadow-sm"
              : "text-muted hover:text-foreground"
          }`}
        >
          <Icon icon={option.icon} className="h-3.5 w-3.5 shrink-0" />
          <span>{option.label}</span>
        </button>
      ))}
    </div>
  )
}

function RepoDiffSection({
  repoDiff,
  options,
}: {
  repoDiff: ReviewRepoDiff
  options: ReviewDiffOptions
}) {
  return (
    <section className="min-w-0 overflow-hidden rounded-lg border border-border bg-background">
      <div className="flex h-9 min-w-0 items-center gap-2 border-b border-border px-3">
        <Icon icon="lucide:git-branch" className="h-3.5 w-3.5 shrink-0 text-muted" />
        <span className="min-w-0 truncate text-sm font-medium">
          {repoDiff.repo.name}
        </span>
        <span className="min-w-0 truncate font-mono text-xs text-muted">
          {repoDiff.repo.relative_path}
        </span>
        <span className="ml-auto shrink-0 text-xs text-muted">
          {formatPatchCount(repoDiff.patches.length)}
        </span>
      </div>
      <div className="flex min-w-0 flex-col">
        {repoDiff.patches.map((patch, index) => (
          <PatchDiff
            key={`${repoDiff.repo.id}:${index}:${patch.slice(0, 48)}`}
            patch={patch}
            options={options}
            className={index > 0 ? "border-t border-border" : undefined}
            style={REVIEW_DIFF_STYLE}
            disableWorkerPool
          />
        ))}
      </div>
    </section>
  )
}

function ReviewLoadingState({ label }: { label: string }) {
  return (
    <div className="flex h-full flex-col gap-4 px-4 py-5">
      <div className="flex items-center gap-2 text-sm text-muted">
        <Spinner size="sm" aria-label={label} />
        {label}
      </div>
      <div className="flex flex-col gap-3">
        {Array.from({ length: 3 }).map((_, index) => (
          <div
            key={index}
            className="overflow-hidden rounded-lg border border-border bg-background"
          >
            <div className="h-9 border-b border-border px-3 py-2">
              <div className="h-3.5 w-36 animate-pulse rounded bg-default" />
            </div>
            <div className="flex flex-col gap-2 p-3">
              {Array.from({ length: 4 }).map((__, lineIndex) => (
                <div
                  key={lineIndex}
                  className="h-3 animate-pulse rounded bg-default"
                  style={{ width: `${60 + ((lineIndex + index) % 4) * 9}%` }}
                />
              ))}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

function ReviewEmptyState({
  icon,
  title,
  message,
}: {
  icon: string
  title: string
  message: string
}) {
  return (
    <div className="flex h-full items-center justify-center px-6 text-center">
      <div className="flex max-w-sm flex-col items-center gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-lg border border-border bg-background">
          <Icon icon={icon} className="h-5 w-5 text-muted" />
        </div>
        <div className="text-sm font-medium">{title}</div>
        <p className="text-sm leading-6 text-muted">{message}</p>
      </div>
    </div>
  )
}

function ReviewErrorState({
  message,
  onRetry,
}: {
  message: string
  onRetry: () => void
}) {
  return (
    <div className="flex h-full items-center justify-center px-6 text-center">
      <div className="flex max-w-sm flex-col items-center gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-lg border border-border bg-background">
          <Icon icon="lucide:circle-alert" className="h-5 w-5 text-muted" />
        </div>
        <div className="text-sm font-medium">Review is not available</div>
        <p className="text-sm leading-6 text-muted">{message}</p>
        <Button size="sm" variant="secondary" onPress={onRetry}>
          Retry
        </Button>
      </div>
    </div>
  )
}

async function fetchReviewDiffs(
  access: RuntimeSandboxAccess,
  signal?: AbortSignal
): Promise<ReviewDiffsResult> {
  const repos = await fetchRuntimeRepos(access, signal)
  const repoDiffs = await Promise.all(
    repos.map(async (repo) => {
      const diff = await fetchRuntimeRepoDiff(access, repo.id, signal)
      return {
        repo,
        patches: splitPatches(diff.diff),
      }
    })
  )
  return { repos, repoDiffs }
}

function splitPatches(diff: string) {
  const clean = diff.trim()
  if (!clean) return []
  const patches = clean.includes("\ndiff --git ")
    ? clean.split(/(?=^diff --git )/m)
    : [clean]
  return patches.map((patch) => `${patch.trimEnd()}\n`).filter(Boolean)
}

function isUnauthorizedRuntimeError(error: unknown) {
  return error instanceof RuntimeRepoHTTPError && error.status === 401
}

function errorMessage(error: unknown, fallback: string) {
  if (error instanceof RuntimeRepoAccessError) return error.message
  if (error instanceof Error && error.message.trim()) return error.message
  return fallback
}

function formatPatchCount(count: number) {
  return `${new Intl.NumberFormat().format(count)} ${
    count === 1 ? "file" : "files"
  }`
}
