import type { CSSProperties } from "react"
import { HIVY_DIFF_STYLE, hivyDiffOptions } from "@/lib/diffs-theme"
import type { ReviewDiffOptions, ReviewDiffStyle } from "./review-types"

export const REVIEW_DIFF_BASE_OPTIONS = hivyDiffOptions({
  overflow: "scroll",
}) satisfies ReviewDiffOptions

export const REVIEW_DIFF_STYLE: CSSProperties & Record<`--${string}`, string> =
  {
    ...HIVY_DIFF_STYLE,
    "--diffs-font-size": "12px",
    "--diffs-line-height": "20px",
  }

export const DIFF_STYLE_OPTIONS: {
  id: ReviewDiffStyle
  label: string
  icon: string
}[] = [
  { id: "unified", label: "Unified", icon: "lucide:rows-3" },
  { id: "split", label: "Split", icon: "lucide:columns-2" },
]
