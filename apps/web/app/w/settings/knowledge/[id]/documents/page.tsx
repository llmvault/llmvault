"use client"

import { useMemo, useState } from "react"
import Link from "next/link"
import { useParams } from "next/navigation"
import { Input } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { ProviderIcon } from "../../_provider-icon"
import {
  providerMeta,
  sourceById,
  STATIC_DOCUMENTS,
} from "../../_data"

export default function KnowledgeDocumentsPage() {
  const params = useParams<{ id: string }>()
  const source = sourceById(params.id)
  const provider = source ? providerMeta(source.provider) : null
  const [query, setQuery] = useState("")

  const documents = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return STATIC_DOCUMENTS
    return STATIC_DOCUMENTS.filter(
      (doc) =>
        doc.title.toLowerCase().includes(q) ||
        doc.link.toLowerCase().includes(q)
    )
  }, [query])

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-3">
        <Link
          href="/w/settings/knowledge"
          className="inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
        >
          <AppIcon icon="arrow-left" className="h-4 w-4" />
          Knowledge
        </Link>
        <div className="flex items-center gap-3">
          {provider ? (
            <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-default">
              <ProviderIcon icon={provider.icon} className="h-5 w-5" />
            </div>
          ) : null}
          <div>
            <h1 className="text-2xl font-semibold text-foreground">
              {source ? source.name : "Documents"}
            </h1>
            <p className="mt-0.5 text-sm text-muted-foreground">
              {STATIC_DOCUMENTS.length} documents ingested
              {source ? ` · ${source.scopeSummary}` : ""}
            </p>
          </div>
        </div>
      </div>

      <div className="relative">
        <AppIcon
          icon="search"
          className="pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2 text-muted-foreground"
        />
        <Input
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          placeholder="Search documents"
          className="h-10 w-full rounded-md bg-card pl-9"
        />
      </div>

      {documents.length === 0 ? (
        <div className="flex min-h-56 flex-col items-center justify-center rounded-xl bg-card px-6 text-center">
          <AppIcon icon="file-text" className="h-7 w-7 text-muted-foreground" />
          <p className="mt-3 text-sm font-medium text-foreground">
            {query ? "No matching documents" : "No documents yet"}
          </p>
          <p className="mt-1 max-w-sm text-sm text-muted-foreground">
            {query
              ? "Try a different search."
              : "Documents appear here once this source finishes its first sync."}
          </p>
        </div>
      ) : (
        <div className="overflow-hidden rounded-xl border border-border">
          {documents.map((doc, index) => (
            <a
              key={doc.id}
              href={doc.link}
              target="_blank"
              rel="noreferrer"
              className={`group flex items-center gap-3 bg-surface px-4 py-3 transition-colors hover:bg-default ${
                index === documents.length - 1 ? "" : "border-b border-border"
              }`}
            >
              <AppIcon
                icon="file-text"
                className="h-4 w-4 shrink-0 text-muted-foreground"
              />
              <div className="flex min-w-0 flex-1 flex-col gap-0.5">
                <span className="truncate text-sm font-medium text-foreground">
                  {doc.title}
                </span>
                <span className="truncate text-xs text-muted-foreground">
                  {doc.link}
                </span>
              </div>
              <span className="shrink-0 text-xs text-muted-foreground">
                {doc.chunkCount} chunks
              </span>
              <span className="hidden shrink-0 text-xs text-muted-foreground sm:inline">
                {doc.updatedAt}
              </span>
              <AppIcon
                icon="external-link"
                className="h-3.5 w-3.5 shrink-0 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100"
              />
            </a>
          ))}
        </div>
      )}
    </div>
  )
}
