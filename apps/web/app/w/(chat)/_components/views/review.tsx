"use client"

import { useMemo } from "react"
import { Button } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { useQuery } from "@tanstack/react-query"
import { Virtualizer } from "@pierre/diffs/react"
import {
  RuntimeRepoHTTPError,
  fetchRuntimeRepoDiff,
  fetchRuntimeRepos,
  type RuntimeSandboxAccess,
} from "@/app/w/(chat)/_lib/runtime-repos"
import { extractErrorMessage as errorMessage } from "@/lib/api/error"
import { reviewDiffsQueryKey } from "@/app/w/(chat)/_lib/review-diffs-query"
import { REVIEW_DIFF_BASE_OPTIONS } from "./review-diff-config"
import { DiffStyleToggle } from "./review-diff-style-toggle"
import { ReviewErrorState } from "./review-error-state"
import { formatPatchCount } from "./review-format"
import { RepoDiffSection } from "./review-repo-diff-section"
import { ReviewEmptyState, ReviewLoadingState } from "./review-state"
import type { ReviewDiffOptions, ReviewDiffsResult } from "./review-types"
import {
  selectSessionWorkspace,
  useSessionWorkspaceStore,
} from "@/app/w/(chat)/_stores/session-workspace-store"

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
  const workspaceSessionId = sessionId ?? "new-chat"
  const diffStyle = useSessionWorkspaceStore(
    (state) =>
      selectSessionWorkspace(state, workspaceSessionId).review.diffStyle
  )
  const setReviewDiffStyle = useSessionWorkspaceStore(
    (state) => state.setReviewDiffStyle
  )
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
        (repoDiff) => repoDiff.files.length > 0
      ) ?? [],
    [reviewQuery.data?.repoDiffs]
  )
  const patchCount = changedRepoDiffs.reduce(
    (total, repoDiff) => total + repoDiff.files.length,
    0
  )

  if (!sessionId) {
    return (
      <ReviewEmptyState
        icon="file-diff"
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
        icon="folder-search"
        title="No repositories found"
        message="The sandbox has no Git repositories under its workspace repos directory."
      />
    )
  }

  return (
    <div className="flex h-full min-w-0 flex-col bg-background">
      <div className="flex h-10 shrink-0 items-center gap-2 border-b border-border px-3">
        <div className="flex min-w-0 flex-1 items-center gap-2">
          <AppIcon
            icon="file-diff"
            className="h-4 w-4 shrink-0 text-muted"
          />
          <span className="truncate text-sm font-medium">Changes</span>
          <span className="shrink-0 text-xs text-muted">
            {formatPatchCount(patchCount)}
          </span>
        </div>
        <DiffStyleToggle
          value={diffStyle}
          onChange={(next) => setReviewDiffStyle(workspaceSessionId, next)}
        />
        <Button
          aria-label="Refresh diffs"
          size="sm"
          variant="ghost"
          isIconOnly
          isDisabled={reviewQuery.isFetching}
          onPress={() => void reviewQuery.refetch()}
        >
          <AppIcon
            icon="refresh-cw"
            className={`h-4 w-4 text-muted ${
              reviewQuery.isFetching ? "animate-spin" : ""
            }`}
          />
        </Button>
      </div>

      {changedRepoDiffs.length > 0 ? (
        <Virtualizer
          className="min-h-0 flex-1 overflow-x-hidden overflow-y-auto bg-surface"
          contentClassName="flex min-h-full min-w-0 flex-col gap-3"
        >
          {changedRepoDiffs.map((repoDiff) => (
            <RepoDiffSection
              key={repoDiff.repo.id}
              repoDiff={repoDiff}
              options={diffOptions}
            />
          ))}
        </Virtualizer>
      ) : (
        <div className="min-h-0 flex-1 overflow-x-hidden overflow-y-auto bg-surface">
          <ReviewEmptyState
            icon="check-circle-2"
            title="No changes to review"
            message="Sandbox repositories currently match their base commits."
          />
        </div>
      )}
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
        truncated: diff.truncated ?? false,
        files: diff.files ?? [],
        message: diff.message,
        totalBytes: diff.total_bytes,
        maxBytes: diff.max_bytes,
      }
    })
  )
  return { repos, repoDiffs }
}

function isUnauthorizedRuntimeError(error: unknown) {
  return error instanceof RuntimeRepoHTTPError && error.status === 401
}
