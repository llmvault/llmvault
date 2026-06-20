import { Spinner } from "@heroui/react"
import { Icon } from "@iconify/react"
import {
  FileMutationDetail,
  ReadFileDetail,
} from "@/app/w/(chat)/_components/tool-file-details"
import {
  statusLabel,
  toolBody,
  toolFailed,
} from "@/app/w/(chat)/_components/tool-block-helpers"
import { WebSearchDetail } from "@/app/w/(chat)/_components/tool-web-search-detail"
import type { ToolCallDetail } from "@/app/w/(chat)/_lib/static-data"

export function ToolDetail({
  detail,
  running,
}: {
  detail: ToolCallDetail
  running?: boolean
}) {
  if (detail.category === "file_read") {
    return <ReadFileDetail detail={detail} />
  }

  if (detail.category === "file_edit" || detail.category === "file_write") {
    return <FileMutationDetail detail={detail} />
  }

  if (detail.category === "web_search" && detail.searchResults?.length) {
    return <WebSearchDetail detail={detail} />
  }

  const body = toolBody(detail)
  return (
    <div className="bg-default rounded-2xl px-4 py-3 text-muted">
      <div className="text-sm">{detail.kind}</div>
      <pre className="mt-7 font-mono text-sm leading-6 break-words whitespace-pre-wrap text-foreground">
        {body}
      </pre>
      {detail.truncated ? (
        <div className="mt-3 text-sm">Output truncated</div>
      ) : null}
      <div className="mt-8 flex items-center justify-end gap-1.5 text-sm">
        {running ? (
          <Spinner color="current" size="sm" />
        ) : (
          <Icon
            icon={toolFailed(detail) ? "lucide:triangle-alert" : "lucide:check"}
            className="h-4 w-4"
          />
        )}
        <span>{statusLabel(detail, running)}</span>
      </div>
    </div>
  )
}
