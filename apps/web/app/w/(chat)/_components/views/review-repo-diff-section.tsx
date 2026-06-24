import { memo, useMemo } from "react"
import { Icon } from "@iconify/react"
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
        <Icon
          icon="lucide:git-branch"
          className="h-3.5 w-3.5 shrink-0 text-muted"
        />
        <span className="min-w-0 truncate text-sm font-medium">
          {repoDiff.repo.name}
        </span>
        <span className="min-w-0 truncate font-mono text-xs text-muted">
          {repoDiff.repo.relative_path}
        </span>
        <span className="ml-auto shrink-0 text-xs text-muted">
          {formatPatchCount(
            repoDiff.truncated ? repoDiff.files.length : repoDiff.patches.length
          )}
        </span>
      </div>
      <div className="flex min-w-0 flex-col">
        {repoDiff.truncated ? (
          <div className="flex min-w-0 flex-col gap-2 px-3 py-3 text-sm">
            <div className="flex min-w-0 items-center gap-2 text-muted">
              <Icon
                icon="lucide:triangle-alert"
                className="text-warning h-4 w-4 shrink-0"
              />
              <span className="min-w-0 truncate">
                {repoDiff.message ?? "Diff is too large to display."}
              </span>
              {typeof repoDiff.totalBytes === "number" &&
              typeof repoDiff.maxBytes === "number" ? (
                <span className="shrink-0 font-mono text-xs">
                  {formatBytes(repoDiff.totalBytes)} /{" "}
                  {formatBytes(repoDiff.maxBytes)}
                </span>
              ) : null}
            </div>
            {repoDiff.files.length > 0 ? (
              <div className="flex min-w-0 flex-col gap-1">
                {repoDiff.files.map((file) => (
                  <div
                    key={`${file.status}:${file.path}`}
                    className="bg-surface-secondary flex min-w-0 items-center gap-2 rounded px-2 py-1.5"
                  >
                    <span className="w-20 shrink-0 text-xs tracking-normal text-muted uppercase">
                      {file.status}
                    </span>
                    <span className="min-w-0 truncate font-mono text-xs">
                      {file.path}
                    </span>
                  </div>
                ))}
              </div>
            ) : null}
          </div>
        ) : null}
        {repoDiff.patches.map((patch, index) => (
          <CommentablePatchDiff
            key={`${repoDiff.repo.id}:${index}:${patch.slice(0, 48)}`}
            patch={patch}
            options={options}
            source={source}
            className={index > 0 ? "border-t border-border" : undefined}
            style={REVIEW_DIFF_STYLE}
            disableWorkerPool
          />
        ))}
      </div>
    </section>
  )
})

function formatBytes(bytes: number) {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 102.4) / 10} KB`
  return `${Math.round(bytes / 1024 / 102.4) / 10} MB`
}
