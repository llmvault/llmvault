import type { CommentablePatchDiffProps } from "@/app/w/(chat)/_components/diff-line-comments"
import type {
  RuntimeRepoDiffFile,
  RuntimeRepoInfo,
} from "@/app/w/(chat)/_lib/runtime-repos"

export type ReviewDiffStyle = "unified" | "split"
export type ReviewDiffOptions = NonNullable<
  CommentablePatchDiffProps["options"]
>

export interface ReviewRepoDiff {
  repo: RuntimeRepoInfo
  truncated: boolean
  files: RuntimeRepoDiffFile[]
  message?: string | null
  totalBytes?: number
  maxBytes?: number
}

export interface ReviewDiffsResult {
  repos: RuntimeRepoInfo[]
  repoDiffs: ReviewRepoDiff[]
}
