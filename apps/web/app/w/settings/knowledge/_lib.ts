import type { components } from "@/lib/api/schema"

export type RagSource = components["schemas"]["ragSourceResponse"]
export type Connection = components["schemas"]["connectionResponse"]

export const RAG_SOURCES_QUERY_KEY = ["get", "/v1/rag/sources"] as const

// Providers we support as knowledge sources, with their display metadata. The
// icon keys resolve through the shared icon registry (brand marks + globe).
export type ProviderMeta = {
  // canonical UI key (used with providerMeta())
  provider: string
  label: string
  icon: string
  // kind sent to POST /v1/rag/sources
  kind: "INTEGRATION" | "WEBSITE"
  // what the user selects to scope this source (used as the label + empty hint)
  scopeNoun: string
  // the actual connection.provider values that map to this source. Only
  // RAG-capable integrations belong here: github-app-code-reviews is excluded
  // because its integration has supports_rag_source=false and the backend
  // rejects it (internal/handler/rag_sources_create.go).
  connectionProviders: string[]
}

export const PROVIDERS: ProviderMeta[] = [
  {
    provider: "github",
    label: "GitHub",
    icon: "github",
    kind: "INTEGRATION",
    scopeNoun: "repositories",
    connectionProviders: ["github-app"],
  },
  { provider: "notion", label: "Notion", icon: "notion", kind: "INTEGRATION", scopeNoun: "pages & databases", connectionProviders: ["notion"] },
  { provider: "slack", label: "Slack", icon: "slack", kind: "INTEGRATION", scopeNoun: "channels", connectionProviders: ["slack"] },
  { provider: "linear", label: "Linear", icon: "linear", kind: "INTEGRATION", scopeNoun: "teams", connectionProviders: ["linear"] },
  { provider: "website", label: "Website", icon: "globe", kind: "WEBSITE", scopeNoun: "sections", connectionProviders: [] },
]

export function providerMeta(provider: string): ProviderMeta {
  return PROVIDERS.find((p) => p.provider === provider) ?? PROVIDERS[0]
}

// metaForConnectionProvider maps a raw connection.provider string (e.g.
// "github-app") to its source meta.
function metaForConnectionProvider(cp?: string | null): ProviderMeta | undefined {
  if (!cp) return undefined
  return PROVIDERS.find((p) => p.connectionProviders.includes(cp))
}

// connectionForProvider returns the org's active connection that backs a source
// provider (matching any of its connection-provider variants).
export function connectionForProvider(
  meta: ProviderMeta,
  connections: Connection[]
): Connection | undefined {
  return connections.find(
    (c) => c.provider && !c.revoked_at && meta.connectionProviders.includes(c.provider)
  )
}

// deriveProvider resolves a source's canonical provider key: WEBSITE sources are
// "website"; INTEGRATION sources map their connection's provider variant.
export function deriveProvider(
  source: RagSource,
  connectionsById: Map<string, Connection>
): string {
  if (source.kind === "WEBSITE") return "website"
  if (source.connection_id) {
    const conn = connectionsById.get(source.connection_id)
    return metaForConnectionProvider(conn?.provider)?.provider ?? "github"
  }
  return "github"
}

export type SourceStatus = "syncing" | "active" | "paused" | "disabled" | "error"

export function deriveStatus(source: RagSource): SourceStatus {
  if (source.enabled === false) return "disabled"
  const attemptStatus = source.latest_attempt?.status
  if (attemptStatus === "in_progress" || attemptStatus === "not_started") {
    return "syncing"
  }
  if (source.in_repeated_error_state || attemptStatus === "failed") {
    return "error"
  }
  if (source.status === "PAUSED") return "paused"
  return "active"
}

export type DerivedProgress = { percent: number | null; label: string }

// deriveProgress maps the backend progress object + status into the bar percent
// and a human label, mirroring the honest-progress model (percent when a
// denominator exists, else an indeterminate running count).
export function deriveProgress(
  source: RagSource,
  status: SourceStatus
): DerivedProgress {
  const progress = source.latest_attempt?.progress
  const docs = progress?.docs_indexed ?? source.total_docs_indexed ?? 0
  const percent =
    progress && !progress.indeterminate && typeof progress.percent === "number"
      ? progress.percent
      : null

  switch (status) {
    case "error":
      return { percent, label: "Last sync failed" }
    case "paused":
      return {
        percent,
        label: percent !== null ? `Paused · ${percent}%` : "Paused",
      }
    case "disabled":
      return { percent: null, label: "Disabled" }
    case "syncing":
      return {
        percent,
        label:
          percent !== null
            ? `Syncing · ${percent}%`
            : `Indexing · ${docs.toLocaleString()} docs`,
      }
    default:
      return {
        percent: 100,
        label: `Up to date · ${(source.total_docs_indexed ?? docs).toLocaleString()} docs`,
      }
  }
}

// scopeSummary describes what a source ingests, read from its config scope
// envelope (integrations) or its url list (website).
export function scopeSummary(source: RagSource, provider: ProviderMeta): string {
  const config = (source.config ?? {}) as Record<string, unknown>
  if (source.kind === "WEBSITE") {
    const urls = Array.isArray(config.urls) ? (config.urls as string[]) : []
    const n = urls.length || (config.url ? 1 : 0)
    return n === 1 ? "1 URL" : `${n} URLs`
  }
  const scope = config.scope as { items?: unknown[] } | undefined
  const n = Array.isArray(scope?.items) ? scope!.items!.length : 0
  if (n === 0) return `All ${provider.scopeNoun}`
  return n === 1 ? `1 ${singular(provider.scopeNoun)}` : `${n} ${provider.scopeNoun}`
}

function singular(noun: string): string {
  if (noun === "repositories") return "repository"
  return noun.replace(/s$/, "")
}
