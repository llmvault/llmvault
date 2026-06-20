"use client"

import { useMemo, useState } from "react"
import { Button } from "@heroui/react"
import { Icon } from "@iconify/react"
import { useQuery } from "@tanstack/react-query"
import {
  RuntimeRepoAccessError,
  RuntimeRepoHTTPError,
  fetchRuntimeRepoDiff,
  fetchRuntimeRepos,
  type RuntimeSandboxAccess,
} from "@/app/w/(chat)/_lib/runtime-repos"
import { reviewDiffsQueryKey } from "@/app/w/(chat)/_lib/review-diffs-query"
import { REVIEW_DIFF_BASE_OPTIONS } from "./review-diff-config"
import { DiffStyleToggle } from "./review-diff-style-toggle"
import { ReviewErrorState } from "./review-error-state"
import { formatPatchCount } from "./review-format"
import { RepoDiffSection } from "./review-repo-diff-section"
import { ReviewEmptyState, ReviewLoadingState } from "./review-state"
import type {
  ReviewDiffOptions,
  ReviewDiffsResult,
  ReviewDiffStyle,
} from "./review-types"

interface ReviewViewProps {
  sessionId?: string
  sandboxAccess?: RuntimeSandboxAccess
  sandboxAccessPending: boolean
  sandboxAccessError: unknown
  onRefreshSandboxAccess: () => void
}

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
    queryKey: reviewDiffsQueryKey(sessionId, sandboxAccess),
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
          <Icon
            icon="lucide:file-diff"
            className="h-4 w-4 shrink-0 text-muted"
          />
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

      <div className="bg-surface min-h-0 flex-1 overflow-x-hidden overflow-y-auto">
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
