import { CommentablePatchDiff } from "@/app/w/(chat)/_components/diff-line-comments"
import {
  displayFileName,
  formatBytes,
  splitPatches,
  TOOL_DIFF_OPTIONS,
} from "@/app/w/(chat)/_components/tool-block-helpers"
import type { ToolCallDetail } from "@/app/w/(chat)/_lib/static-data"
import { HIVY_DIFF_STYLE } from "@/lib/diffs-theme"

export function ReadFileDetail({ detail }: { detail: ToolCallDetail }) {
  const files = detail.paths?.length
    ? detail.paths
    : detail.path
      ? [detail.path]
      : []

  return (
    <div className="flex flex-col gap-2 pt-1 text-sm text-muted">
      {files.length > 0 ? (
        files.map((file) => (
          <div key={file} className="min-w-0 truncate">
            Read{" "}
            <span className="font-mono text-[14px]">
              {displayFileName(file)}
            </span>
          </div>
        ))
      ) : (
        <div>Read a file</div>
      )}
    </div>
  )
}

export function FileMutationDetail({ detail }: { detail: ToolCallDetail }) {
  const patches = detail.diff ? splitPatches(detail.diff) : []
  const file = detail.path ?? detail.paths?.[0]
  const title = detail.category === "file_write" ? "Wrote file" : "Edited file"

  return (
    <div className="flex min-w-0 flex-col gap-3">
      <div className="flex items-center gap-2 text-sm text-muted">
        <span>{title}</span>
        {file ? (
          <span className="min-w-0 truncate font-mono text-[14px]">
            {displayFileName(file)}
          </span>
        ) : null}
        {typeof detail.editsApplied === "number" ? (
          <span>{detail.editsApplied} edits</span>
        ) : null}
        {typeof detail.bytesWritten === "number" ? (
          <span>{formatBytes(detail.bytesWritten)}</span>
        ) : null}
      </div>

      {patches.length > 0 ? (
        <div className="flex min-w-0 flex-col gap-3 overflow-hidden rounded-2xl border border-border bg-background">
          {patches.map((patch, index) => (
            <CommentablePatchDiff
              key={`${index}-${patch.slice(0, 32)}`}
              patch={patch}
              options={TOOL_DIFF_OPTIONS}
              source={{
                kind: "tool",
                path: file,
              }}
              style={HIVY_DIFF_STYLE}
              disableWorkerPool
            />
          ))}
        </div>
      ) : file ? (
        <div className="bg-default rounded-2xl px-4 py-3 font-mono text-[14px] text-muted">
          {file}
        </div>
      ) : null}
    </div>
  )
}
