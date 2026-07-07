import { AppIcon } from "@/components/icon"
import { IntegrationLogo } from "@/components/integration-logo"
import type { KnowledgeSearchSummaryLine } from "@/app/w/(chat)/_lib/knowledge-search-summary"

/**
 * Grouped, logo-led summary for `search_knowledge_base` tool results: one
 * line per source kind ("Found 2 Slack messages"). The parsing/grouping
 * lives in _lib/knowledge-search-summary.ts; callers fall back to the raw
 * JSON rendering when that parser returns null.
 */
export function KnowledgeSearchDetail({
  lines,
}: {
  lines: KnowledgeSearchSummaryLine[]
}) {
  if (lines.length === 0) {
    return <div className="text-sm text-muted">No knowledge base results</div>
  }

  return (
    <div className="flex min-w-0 flex-col gap-1.5 text-sm">
      {lines.map((line) => (
        <div
          key={line.key}
          className="flex min-w-0 items-center gap-2 text-muted"
        >
          {line.provider ? (
            <IntegrationLogo provider={line.provider} size={16} />
          ) : (
            <AppIcon
              icon={line.icon ?? "database"}
              className="h-4 w-4 shrink-0"
            />
          )}
          <span className="min-w-0 flex-1 truncate">{line.text}</span>
        </div>
      ))}
    </div>
  )
}
