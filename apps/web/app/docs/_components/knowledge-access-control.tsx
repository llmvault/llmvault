import Link from "next/link"
import type { ReactNode } from "react"
import { AppIcon } from "@/components/icon"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

const ACCESS_PATH = [
  {
    number: "01",
    title: "Source scope",
    description:
      "Sets the repositories, pages, channels, Linear teams, or website URLs that enter the index.",
  },
  {
    number: "02",
    title: "Team grant",
    description: "Gives one team's agents permission to search the source.",
  },
  {
    number: "03",
    title: "Channel",
    description: "Uses its team's approved sources in every session.",
  },
]

const SOURCE_STATES = [
  [
    "Indexing",
    "Hivy is processing documents from the first sync or a later update.",
  ],
  [
    "Up to date",
    "Agents can search the indexed content, and the latest sync succeeded.",
  ],
  [
    "Paused",
    "Hivy will not run scheduled indexing until an admin resumes the source.",
  ],
  [
    "Disabled",
    "Hivy skips this source during scheduled indexing until an admin enables it.",
  ],
  [
    "Last sync failed",
    "Hivy could not finish the latest sync. Check the connection and source scope before trying again.",
  ],
]

export function KnowledgeAccessControl() {
  return (
    <div className="mt-10 text-base leading-7">
      <p className="max-w-2xl text-muted">
        Knowledge access has two gates. First, choose what enters the index;
        then grant the source to specific teams. Without a grant, a team&apos;s
        agents get no results from that source.
      </p>

      <section aria-labelledby="knowledge-access-path" className="mt-14">
        <h2
          id="knowledge-access-path"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          Access follows the team
        </h2>
        <p className="mt-3 max-w-2xl text-muted">
          Sources belong to teams, not individual sessions. Every channel uses
          the sources granted to its team, which keeps one access boundary in
          place for all agents working there.
        </p>
        <ol className="mt-8 divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
          {ACCESS_PATH.map((item) => (
            <li key={item.title} className="p-5">
              <span className="text-xs font-semibold tracking-[0.12em] text-muted uppercase">
                {item.number}
              </span>
              <h3 className="mt-4 font-semibold text-foreground">
                {item.title}
              </h3>
              <p className="mt-2 text-sm leading-6 text-muted">
                {item.description}
              </p>
            </li>
          ))}
        </ol>
      </section>

      <DocsMediaPlaceholder
        className="mt-12"
        type="image"
        title="Source scope and team grants"
        description="Capture Edit source with the provider, selected resources, and Teams field in the same frame. Use demo names that show one narrowly scoped source granted to a single team; keep every label readable."
      />

      <div className="mt-16 space-y-14 border-t border-border pt-14">
        <DocSection title="Limit what enters the index">
          <p>
            A connection may read far more than an agent needs. Keep each source
            to the smallest useful set of repositories, pages, databases,
            channels, Linear teams, or website URLs because everything in one
            source shares the same team grants.
          </p>
          <p className="mt-3">
            If an admin removes a resource from the scope, Hivy deletes that
            resource from the index. Hivy reads newly added resources from the
            beginning before agents can search them.
          </p>
        </DocSection>

        <DocSection title="Grant access by team">
          <p>
            Open the source, choose <strong>Edit source</strong>, then update
            the <strong>Teams</strong> field. A grant makes the source
            searchable in every channel owned by that team, while revoking it
            leaves other teams unchanged.
          </p>
          <p className="mt-3">
            On every search, Hivy checks the active session&apos;s channel and
            its team grants. The agent cannot switch the channel behind the
            search or cross that boundary.
          </p>
          <DocLink href="/w/settings/knowledge">
            Open Knowledge settings
          </DocLink>
        </DocSection>

        <DocsMediaPlaceholder
          type="video"
          title="Change knowledge access safely"
          description="Capture an admin opening Edit source, removing one resource, adding another team, and saving. Open View documents to check the remaining index. Finish with a session in a team that has no grant and show a search that returns nothing from this source."
          bleed={false}
        />

        <DocSection title="Treat team grants as the access switch">
          <p>
            <strong>Pause ingestion</strong> and <strong>Disable source</strong>
            stop scheduled indexing; neither action revokes a team grant. Remove
            the team from <strong>Teams</strong> when its agents must lose
            search access. Use <strong>Remove source</strong> when the workspace
            should no longer keep its indexed documents.
          </p>
          <DocLink href="/docs/workspace-and-access/teams">
            Read how teams control resources
          </DocLink>
        </DocSection>

        <DocSection title="Check sync health before trusting an answer">
          <dl className="divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
            {SOURCE_STATES.map(([state, meaning]) => (
              <div
                key={state}
                className="grid gap-1 px-4 py-4 sm:grid-cols-[9rem_1fr] sm:gap-5"
              >
                <dt className="text-sm font-semibold text-foreground">
                  {state}
                </dt>
                <dd className="text-sm leading-6 text-muted">{meaning}</dd>
              </div>
            ))}
          </dl>
          <p className="mt-4">
            Connected sources refresh on a schedule. If indexing still runs or
            the latest sync failed, search may use an incomplete or older index.
            For current provider data, use its live plugin.
          </p>
        </DocSection>

        <DocSection title="Inspect what Hivy indexed">
          <p>
            Choose <strong>View documents</strong> from the source menu. Search
            the document titles, links, and update dates to find anything
            missing or outside the intended scope.
          </p>
        </DocSection>

        <section
          aria-labelledby="knowledge-review-checklist"
          className="rounded-xl border border-border bg-surface-secondary p-6"
        >
          <h2
            id="knowledge-review-checklist"
            className="text-lg font-semibold tracking-tight text-foreground"
          >
            Audit a source before agents use it
          </h2>
          <ul className="mt-4 space-y-3 text-sm text-muted">
            {[
              "Check the content the connection can read.",
              "Cut the source scope down to what agents need.",
              "Grant the source only to teams that own this work.",
              "Inspect View documents, then test a search from one granted channel and one ungranted channel.",
            ].map((item) => (
              <li key={item} className="flex gap-3">
                <AppIcon
                  icon="check"
                  className="mt-1 h-4 w-4 shrink-0 text-accent"
                />
                <span>{item}</span>
              </li>
            ))}
          </ul>
        </section>
      </div>
    </div>
  )
}

function DocSection({
  title,
  children,
}: {
  title: string
  children: ReactNode
}) {
  const id = title.toLowerCase().replaceAll(" ", "-")

  return (
    <section aria-labelledby={id}>
      <h2
        id={id}
        className="text-xl font-semibold tracking-tight text-foreground"
      >
        {title}
      </h2>
      <div className="mt-3 max-w-2xl text-muted">{children}</div>
    </section>
  )
}

function DocLink({ href, children }: { href: string; children: ReactNode }) {
  return (
    <Link
      href={href}
      className="mt-5 inline-flex items-center gap-2 rounded-sm text-sm font-medium text-foreground transition-colors hover:text-accent focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-focus"
    >
      {children}
      <AppIcon icon="arrow-right" className="h-4 w-4" />
    </Link>
  )
}
