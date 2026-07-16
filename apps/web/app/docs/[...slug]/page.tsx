import type { Metadata } from "next"
import Link from "next/link"
import { notFound } from "next/navigation"
import type { ComponentType } from "react"
import { AppIcon } from "@/components/icon"
import { AccessControl } from "../_components/access-control"
import { AgentCatalog } from "../_components/agent-catalog"
import { AgentBuiltApps } from "../_components/agent-built-apps"
import { AgentSessions } from "../_components/agent-sessions"
import { AutomationRuns } from "../_components/automation-runs"
import { AutomationsOverview } from "../_components/automations-overview"
import { Channels } from "../_components/channels"
import { ConnectionsAccess } from "../_components/connections-access"
import { ConnectTools } from "../_components/connect-tools"
import { ConfigureAgent } from "../_components/configure-agent"
import { EventTriggers } from "../_components/event-triggers"
import { GeneratedWork } from "../_components/generated-work"
import { HowHivyOrganizesWork } from "../_components/how-hivy-organizes-work"
import { HttpWebhooks } from "../_components/http-webhooks"
import { KnowledgeAccessControl } from "../_components/knowledge-access-control"
import { KnowledgeSources } from "../_components/knowledge-sources"
import { MemoriesAndRules } from "../_components/memories-and-rules"
import { RunFirstAgent } from "../_components/run-first-agent"
import { Schedules } from "../_components/schedules"
import { Sheets } from "../_components/sheets"
import { SlackChannels } from "../_components/slack-channels"
import { Teams } from "../_components/teams"
import { ToolsAndSubAgents } from "../_components/tools-and-sub-agents"
import { UsageBilling } from "../_components/usage-billing"
import { WelcomeToHivy } from "../_components/welcome-to-hivy"
import { DOC_PAGES, getAdjacentDocPages, getDocPage } from "../_lib/navigation"

type DocsPageProps = {
  params: Promise<{ slug: string[] }>
}

const DOC_PAGE_COMPONENTS: Partial<Record<string, ComponentType>> = {
  "welcome-to-hivy": WelcomeToHivy,
  "run-your-first-agent": RunFirstAgent,
  "how-hivy-organizes-work": HowHivyOrganizesWork,
  "workspace-and-access/teams": Teams,
  "workspace-and-access/channels": Channels,
  "workspace-and-access/roles-and-invitations": AccessControl,
  "workspace-and-access/slack-channels": SlackChannels,
  "agents/agent-catalog": AgentCatalog,
  "agents/configure-an-agent": ConfigureAgent,
  "agents/tools-and-sub-agents": ToolsAndSubAgents,
  "agents/sessions": AgentSessions,
  "agents/generated-work": GeneratedWork,
  "connections-and-skills/how-access-works": ConnectionsAccess,
  "connections-and-skills/connect-tools": ConnectTools,
  "knowledge-and-memory/knowledge-sources": KnowledgeSources,
  "knowledge-and-memory/indexing-and-access": KnowledgeAccessControl,
  "knowledge-and-memory/memories-and-rules": MemoriesAndRules,
  "automations/overview": AutomationsOverview,
  "automations/event-triggers": EventTriggers,
  "automations/schedules": Schedules,
  "automations/http-webhooks": HttpWebhooks,
  "automations/runs-and-troubleshooting": AutomationRuns,
  "sheets-and-apps/sheets": Sheets,
  "sheets-and-apps/agent-built-apps": AgentBuiltApps,
  "administration/usage-and-billing": UsageBilling,
}

export function generateStaticParams() {
  return DOC_PAGES.map((page) => ({ slug: page.slug.split("/") }))
}

export async function generateMetadata({
  params,
}: DocsPageProps): Promise<Metadata> {
  const { slug } = await params
  const page = getDocPage(slug.join("/"))
  if (!page) return {}
  return { title: page.title, description: page.description }
}

export default async function DocsPage({ params }: DocsPageProps) {
  const { slug } = await params
  const page = getDocPage(slug.join("/"))
  if (!page) notFound()

  const adjacent = getAdjacentDocPages(page.slug)
  const PageBody = DOC_PAGE_COMPONENTS[page.slug]

  return (
    <article className="mx-auto flex min-h-[calc(100dvh-11.5rem)] w-full max-w-2xl flex-col">
      <header className="max-w-2xl">
        <Link
          href="/docs"
          className="inline-flex items-center gap-1.5 rounded-sm text-sm font-medium text-muted transition-colors hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus"
        >
          Documentation
          <AppIcon icon="chevron-right" className="h-3.5 w-3.5" />
          <span className="text-foreground">{page.section}</span>
        </Link>
        <h1 className="mt-4 text-3xl leading-tight font-semibold tracking-tight text-foreground sm:text-4xl">
          {page.title}
        </h1>
        <p className="mt-4 max-w-2xl text-base leading-7 text-muted">
          {page.description}
        </p>
      </header>

      {PageBody ? (
        <PageBody />
      ) : (
        <div className="min-h-64 flex-1" aria-hidden="true" />
      )}

      <nav
        aria-label="Previous and next documentation pages"
        className={`flex flex-col gap-4 border-t border-border pt-8 ${
          PageBody ? "mt-16" : ""
        }`}
      >
        {adjacent.previous ? (
          <PageDirectionLink page={adjacent.previous} direction="previous" />
        ) : null}
        {adjacent.next ? (
          <PageDirectionLink page={adjacent.next} direction="next" />
        ) : null}
      </nav>
    </article>
  )
}

function PageDirectionLink({
  page,
  direction,
}: {
  page: (typeof DOC_PAGES)[number]
  direction: "previous" | "next"
}) {
  const next = direction === "next"
  return (
    <Link
      href={`/docs/${page.slug}`}
      className={`group rounded-lg border border-border bg-surface p-4 transition-colors hover:bg-surface-secondary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus ${
        next ? "text-right" : "text-left"
      }`}
    >
      <span
        className={`flex items-center gap-1.5 text-xs font-medium text-muted ${next ? "justify-end" : ""}`}
      >
        {!next ? <AppIcon icon="arrow-left" className="h-3.5 w-3.5" /> : null}
        {next ? "Next" : "Previous"}
        {next ? <AppIcon icon="arrow-right" className="h-3.5 w-3.5" /> : null}
      </span>
      <span className="mt-2 block text-sm font-medium text-foreground group-hover:text-accent">
        {page.title}
      </span>
    </Link>
  )
}
