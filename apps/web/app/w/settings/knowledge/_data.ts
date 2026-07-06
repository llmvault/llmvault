// Static design data for the Knowledge settings pages. Replace with the
// /v1/rag/* API (sources, attempts/progress, channel scopes, documents) once
// the UI is wired up. Kept in one module so the pages stay presentational.

export type Provider = "github" | "notion" | "slack" | "website"

export type SourceStatus =
  | "syncing"
  | "active"
  | "paused"
  | "disabled"
  | "error"

export type KnowledgeSource = {
  id: string
  name: string
  provider: Provider
  scopeSummary: string
  channel: string
  status: SourceStatus
  // percent === null → indeterminate (running count, no known total).
  progress: { percent: number | null; label: string }
  docCount: number
  lastSynced: string
}

export type ProviderMeta = {
  id: Provider
  label: string
  icon: string
  // What the user selects to scope this source.
  resourceLabel: string
  resourceHint: string
  resources: { id: string; name: string }[]
}

export const PROVIDERS: ProviderMeta[] = [
  {
    id: "github",
    label: "GitHub",
    icon: "github",
    resourceLabel: "Repositories",
    resourceHint: "Choose which repositories this source ingests.",
    resources: [
      { id: "acme/web", name: "acme/web" },
      { id: "acme/api", name: "acme/api" },
      { id: "acme/docs", name: "acme/docs" },
      { id: "acme/infra", name: "acme/infra" },
      { id: "acme/mobile", name: "acme/mobile" },
    ],
  },
  {
    id: "notion",
    label: "Notion",
    icon: "notion",
    resourceLabel: "Pages & databases",
    resourceHint: "Choose the Notion pages or databases (and everything nested under them).",
    resources: [
      { id: "pg-eng-wiki", name: "Engineering Wiki" },
      { id: "pg-product", name: "Product Docs" },
      { id: "pg-onboarding", name: "Onboarding" },
      { id: "db-decisions", name: "Decision Log (database)" },
      { id: "db-runbooks", name: "Runbooks (database)" },
    ],
  },
  {
    id: "slack",
    label: "Slack",
    icon: "slack",
    resourceLabel: "Channels",
    resourceHint: "Choose which Slack channels this source ingests.",
    resources: [
      { id: "c-engineering", name: "#engineering" },
      { id: "c-support", name: "#customer-support" },
      { id: "c-product", name: "#product" },
      { id: "c-general", name: "#general" },
    ],
  },
  {
    id: "website",
    label: "Website",
    icon: "globe",
    resourceLabel: "Sections",
    resourceHint: "Choose which sections of the site to crawl.",
    resources: [
      { id: "docs", name: "/docs" },
      { id: "blog", name: "/blog" },
      { id: "pricing", name: "/pricing" },
      { id: "changelog", name: "/changelog" },
    ],
  },
]

export function providerMeta(provider: Provider): ProviderMeta {
  return PROVIDERS.find((p) => p.id === provider) ?? PROVIDERS[0]
}

export const CHANNELS: { id: string; name: string }[] = [
  { id: "c-engineering", name: "engineering" },
  { id: "c-support", name: "customer-support" },
  { id: "c-product", name: "product" },
  { id: "c-general", name: "general" },
]

export const STATIC_SOURCES: KnowledgeSource[] = [
  {
    id: "src-eng-repos",
    name: "Engineering repos",
    provider: "github",
    scopeSummary: "3 repositories",
    channel: "engineering",
    status: "syncing",
    progress: { percent: 62, label: "62% · 124 / 200 docs" },
    docCount: 124,
    lastSynced: "syncing now",
  },
  {
    id: "src-eng-wiki",
    name: "Engineering wiki",
    provider: "notion",
    scopeSummary: "Engineering Wiki + 1 database",
    channel: "engineering",
    status: "active",
    progress: { percent: 100, label: "Up to date · 318 docs" },
    docCount: 318,
    lastSynced: "12 min ago",
  },
  {
    id: "src-support-slack",
    name: "Support conversations",
    provider: "slack",
    scopeSummary: "#customer-support",
    channel: "customer-support",
    status: "syncing",
    progress: { percent: null, label: "Indexing · 1,204 docs so far" },
    docCount: 1204,
    lastSynced: "syncing now",
  },
  {
    id: "src-product-docs",
    name: "Product docs site",
    provider: "website",
    scopeSummary: "/docs · up to 100 pages",
    channel: "product",
    status: "paused",
    progress: { percent: 45, label: "Paused · 45% · 45 / 100 pages" },
    docCount: 45,
    lastSynced: "2 hours ago",
  },
  {
    id: "src-decisions",
    name: "Decision log",
    provider: "notion",
    scopeSummary: "Decision Log (database)",
    channel: "product",
    status: "error",
    progress: { percent: null, label: "Last sync failed" },
    docCount: 87,
    lastSynced: "1 day ago",
  },
]

export function sourceById(id: string): KnowledgeSource | undefined {
  return STATIC_SOURCES.find((s) => s.id === id)
}

export type KnowledgeDocument = {
  id: string
  title: string
  link: string
  chunkCount: number
  updatedAt: string
}

export const STATIC_DOCUMENTS: KnowledgeDocument[] = [
  {
    id: "d1",
    title: "Deploy runbook",
    link: "https://github.com/acme/infra/blob/main/docs/deploy.md",
    chunkCount: 6,
    updatedAt: "3 days ago",
  },
  {
    id: "d2",
    title: "Incident response policy",
    link: "https://github.com/acme/infra/blob/main/docs/incidents.md",
    chunkCount: 4,
    updatedAt: "1 week ago",
  },
  {
    id: "d3",
    title: "Service catalog",
    link: "https://github.com/acme/infra/blob/main/docs/services.md",
    chunkCount: 11,
    updatedAt: "2 weeks ago",
  },
  {
    id: "d4",
    title: "On-call rotation",
    link: "https://github.com/acme/infra/blob/main/docs/oncall.md",
    chunkCount: 2,
    updatedAt: "yesterday",
  },
]
