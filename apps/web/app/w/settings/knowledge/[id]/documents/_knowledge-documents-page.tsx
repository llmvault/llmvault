"use client"

import { useEffect, useMemo, useState } from "react"
import Link from "next/link"
import { useParams, useRouter } from "next/navigation"
import { Input, Skeleton } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { $api } from "@/lib/api/hooks"
import { useAuth } from "@/lib/auth/auth-context"
import { useIsAdmin } from "@/lib/auth/use-role"
import { ProviderIcon } from "../../_provider-icon"
import {
  deriveProvider,
  providerMeta,
  scopeSummary,
  type Connection,
} from "../../_lib"

const EMPTY_CONNECTIONS: Connection[] = []

export default function KnowledgeDocumentsPageContent() {
  const params = useParams<{ id: string }>()
  const router = useRouter()
  const isAdmin = useIsAdmin()
  const { isLoading: authLoading } = useAuth()
  const id = params.id
  const [query, setQuery] = useState("")

  const sourceQuery = $api.useQuery(
    "get",
    "/v1/rag/sources/{id}",
    { params: { path: { id } } },
    { enabled: isAdmin }
  )
  const connectionsQuery = $api.useQuery(
    "get",
    "/v1/connections",
    { params: { query: { limit: 100 } } },
    { enabled: isAdmin }
  )
  const documentsQuery = $api.useQuery(
    "get",
    "/v1/rag/sources/{id}/documents",
    { params: { path: { id }, query: { limit: 200 } } },
    { enabled: isAdmin }
  )

  useEffect(() => {
    if (!authLoading && !isAdmin) router.replace("/w/teams")
  }, [authLoading, isAdmin, router])

  const source = sourceQuery.data
  const connections = connectionsQuery.data?.data ?? EMPTY_CONNECTIONS
  const provider = useMemo(() => {
    if (!source) return null
    const map = new Map<string, Connection>()
    for (const c of connections) if (c.id) map.set(c.id, c)
    return providerMeta(deriveProvider(source, map))
  }, [source, connections])

  const allDocs = useMemo(
    () => documentsQuery.data?.documents ?? [],
    [documentsQuery.data]
  )
  const documents = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return allDocs
    return allDocs.filter(
      (doc) =>
        (doc.title ?? "").toLowerCase().includes(q) ||
        (doc.link ?? "").toLowerCase().includes(q)
    )
  }, [allDocs, query])

  if (!isAdmin) return null

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-3">
        <Link
          href="/w/knowledge"
          className="inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
        >
          <AppIcon icon="arrow-left" className="h-4 w-4" />
          Knowledge
        </Link>
        <div className="flex items-center gap-3">
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-default">
            {provider ? (
              <ProviderIcon icon={provider.icon} className="h-5 w-5" />
            ) : (
              <AppIcon icon="file-text" className="h-5 w-5 text-muted-foreground" />
            )}
          </div>
          <div>
            <h1 className="text-lg font-semibold text-foreground">
              {source?.name ?? "Documents"}
            </h1>
            <p className="mt-0.5 text-sm text-muted-foreground">
              {allDocs.length}
              {documentsQuery.data?.next_cursor ? "+" : ""} documents ingested
              {source && provider ? ` · ${scopeSummary(source, provider)}` : ""}
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

      {documentsQuery.isLoading ? (
        <DocumentsSkeleton />
      ) : documentsQuery.isError ? (
        <StateCard icon="circle-alert" title="Couldn’t load documents" hint="Something went wrong fetching this source’s documents." danger />
      ) : documents.length === 0 ? (
        <StateCard
          icon="file-text"
          title={query ? "No matching documents" : "No documents yet"}
          hint={
            query
              ? "Try a different search."
              : "Documents appear here once this source finishes its first sync."
          }
        />
      ) : (
        <div className="overflow-hidden rounded-xl border border-border">
          {documents.map((doc, index) => (
            <a
              key={doc.doc_id ?? index}
              href={doc.link}
              target="_blank"
              rel="noreferrer"
              className={`group flex items-center gap-3 bg-surface px-4 py-3 transition-colors hover:bg-default ${
                index === documents.length - 1 ? "" : "border-b border-border"
              }`}
            >
              <AppIcon icon="file-text" className="h-4 w-4 shrink-0 text-muted-foreground" />
              <div className="flex min-w-0 flex-1 flex-col gap-0.5">
                <span className="truncate text-sm font-medium text-foreground">
                  {doc.title || doc.doc_id}
                </span>
                <span className="truncate text-xs text-muted-foreground">{doc.link}</span>
              </div>
              <span className="hidden shrink-0 text-xs text-muted-foreground sm:inline">
                {formatUpdated(doc.doc_updated_at)}
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

function formatUpdated(unix?: number): string {
  if (!unix) return ""
  return new Date(unix * 1000).toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  })
}

function DocumentsSkeleton() {
  return (
    <div className="overflow-hidden rounded-xl border border-border">
      {[0, 1, 2, 3].map((i) => (
        <div key={i} className="flex items-center gap-3 border-b border-border bg-surface px-4 py-3 last:border-b-0">
          <Skeleton className="h-4 w-4 rounded" />
          <div className="flex flex-1 flex-col gap-1">
            <Skeleton className="h-4 w-48 rounded" />
            <Skeleton className="h-3 w-72 rounded" />
          </div>
        </div>
      ))}
    </div>
  )
}

function StateCard({
  icon,
  title,
  hint,
  danger,
}: {
  icon: string
  title: string
  hint: string
  danger?: boolean
}) {
  return (
    <div className="flex min-h-56 flex-col items-center justify-center rounded-xl bg-card px-6 text-center">
      <AppIcon icon={icon} className={`h-7 w-7 ${danger ? "text-danger" : "text-muted-foreground"}`} />
      <p className="mt-3 text-sm font-medium text-foreground">{title}</p>
      <p className="mt-1 max-w-sm text-sm text-muted-foreground">{hint}</p>
    </div>
  )
}
