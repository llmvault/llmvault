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
          {formatPatchCount(repoDiff.patches.length)}
        </span>
      </div>
      <div className="flex min-w-0 flex-col">
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
