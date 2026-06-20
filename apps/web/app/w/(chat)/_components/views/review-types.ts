import type { CommentablePatchDiffProps } from "@/app/w/(chat)/_components/diff-line-comments"
import type { RuntimeRepoInfo } from "@/app/w/(chat)/_lib/runtime-repos"

export type ReviewDiffStyle = "unified" | "split"
export type ReviewDiffOptions = NonNullable<
  CommentablePatchDiffProps["options"]
>

export interface ReviewRepoDiff {
  repo: RuntimeRepoInfo
  patches: string[]
}

export interface ReviewDiffsResult {
  repos: RuntimeRepoInfo[]
  repoDiffs: ReviewRepoDiff[]
}
