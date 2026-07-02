import { memo, useMemo } from "react"
import { AppIcon } from "@/components/icon"
import { CommentablePatchDiff } from "@/app/w/(chat)/_components/diff-line-comments"
import { REVIEW_DIFF_STYLE } from "./review-diff-config"
import { formatPatchCount } from "./review-format"
import type { ReviewDiffOptions, ReviewRepoDiff } from "./review-types"

export const RepoDiffSection = memo(function RepoDiffSection({
  repoDiff,
  options,
}: {
  repoDiff: ReviewRepoDiff
  options: ReviewDiffOptions
}) {
  const source = useMemo(
    () => ({
      kind: "review" as const,
      repoId: repoDiff.repo.id,
      repoName: repoDiff.repo.name,
      repoPath: repoDiff.repo.relative_path,
    }),
    [repoDiff.repo.id, repoDiff.repo.name, repoDiff.repo.relative_path]
  )

  return (
    <section className="min-w-0 overflow-hidden rounded-lg border border-border bg-background">
      <div className="flex h-9 min-w-0 items-center gap-2 border-b border-border px-3">
        <AppIcon
          icon="git-branch"
          className="h-3.5 w-3.5 shrink-0 text-muted"
        />
        <span className="min-w-0 truncate text-sm font-medium">
          {repoDiff.repo.name}
        </span>
        <span className="min-w-0 truncate font-mono text-xs text-muted">
          {repoDiff.repo.relative_path}
        </span>
        <span className="ml-auto shrink-0 text-xs text-muted">
          {formatPatchCount(repoDiff.files.length)}
        </span>
      </div>
      <div className="flex min-w-0 flex-col">
        {repoDiff.files.map((file, index) =>
          file.truncated ? (
            <div
              key={`${file.status}:${file.path}`}
              className={`flex min-w-0 items-center gap-2 px-3 py-3 text-sm text-muted ${
                index > 0 ? "border-t border-border" : ""
              }`}
            >
              <AppIcon
                icon="triangle-alert"
                className="h-4 w-4 shrink-0 text-warning"
              />
              <span className="w-20 shrink-0 text-xs tracking-normal uppercase">
                {file.status}
              </span>
              <span className="min-w-0 flex-1 truncate font-mono text-xs">
                {file.path}
              </span>
              <span className="min-w-0 truncate text-xs">
                {file.message ?? "File diff is too large to display."}
              </span>
              <span className="shrink-0 font-mono text-xs">
                {formatBytes(file.total_bytes)} / {formatBytes(file.max_bytes)}
              </span>
            </div>
          ) : file.patch.trim() ? (
            <CommentablePatchDiff
              key={`${file.status}:${file.path}`}
              patch={file.patch}
              options={options}
              source={source}
              className={index > 0 ? "border-t border-border" : undefined}
              style={REVIEW_DIFF_STYLE}
              disableWorkerPool
            />
          ) : null
        )}
      </div>
    </section>
  )
})

function formatBytes(bytes: number) {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 102.4) / 10} KB`
  return `${Math.round(bytes / 1024 / 102.4) / 10} MB`
}
