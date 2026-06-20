import { Link } from "@heroui/react"
import type { ToolCallDetail } from "@/app/w/(chat)/_lib/static-data"

export function WebSearchDetail({ detail }: { detail: ToolCallDetail }) {
  const results = detail.searchResults ?? []

  return (
    <div className="flex min-w-0 flex-col gap-1 text-sm">
      {results.map((result, index) => (
        <Link
          key={`${result.url}-${index}`}
          href={result.url}
          target="_blank"
          rel="noreferrer"
          className="block min-w-0 truncate font-mono text-[13px] text-muted underline-offset-2 hover:text-foreground hover:underline"
        >
          {result.url}
        </Link>
      ))}
    </div>
  )
}
