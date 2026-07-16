import Link from "next/link"
import type { ReactNode } from "react"
import { AppIcon } from "@/components/icon"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

const ACCESS_LAYERS = [
  {
    number: "01",
    title: "Connection",
    description:
      "One workspace account signs in to an external service; owners and admins look after the connection.",
  },
  {
    number: "02",
    title: "Team grant",
    description:
      "An admin grants one concrete connection instance to a team. Every agent on that team can use its MCP server.",
  },
  {
    number: "03",
    title: "Generated MCP tools",
    description:
      "The connection exposes its capabilities directly as generated MCP tools; no bundled skill is installed.",
  },
]

const ACCESS_ROLES = [
  [
    "Workspace owners and admins",
    "They create external account connections, control resources, grant connection instances to teams, and manage org-wide skills.",
  ],
  [
    "Team members",
    "They can inspect their teams' connections and create, edit, or archive team-owned skills.",
  ],
  [
    "Agents",
    "Agents receive team-owned skills, directly granted org skills, and generated MCP tools from their team's connections.",
  ],
]

export function ConnectionsAccess() {
  return (
    <div className="mt-10 text-base leading-7">
      <p className="max-w-2xl text-muted">
        Create as many connection instances as the workspace needs, then grant
        each concrete instance to the teams that may use it. You don&apos;t need
        a separate sign-in for every agent.
      </p>

      <section aria-labelledby="four-layers-of-access" className="mt-14">
        <h2
          id="four-layers-of-access"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          Follow the three layers of access
        </h2>
        <p className="mt-3 max-w-2xl text-muted">
          A connection alone doesn&apos;t open an external service to an agent.
          Hivy checks the connection instance and team grant whenever an agent
          calls its generated MCP server.
        </p>

        <ol className="mt-8 divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
          {ACCESS_LAYERS.map((layer) => (
            <li key={layer.title} className="p-5">
              <span className="text-xs font-semibold tracking-[0.12em] text-muted uppercase">
                {layer.number}
              </span>
              <h3 className="mt-4 font-semibold text-foreground">
                {layer.title}
              </h3>
              <p className="mt-2 text-sm leading-6 text-muted">
                {layer.description}
              </p>
            </li>
          ))}
        </ol>
      </section>

      <DocsMediaPlaceholder
        className="mt-12"
        type="video"
        title="Trace an agent's tool access"
        description="Record an admin creating a second provider connection, granting that exact instance to a team, and showing its generated MCP tools becoming available."
      />

      <div className="mt-16 space-y-14 border-t border-border pt-14">
        <DocSection title="Connections are workspace-level instances">
          <p>
            Hivy calls the approved link to an external account a connection.
            Slack, GitHub, Notion, and Google Drive connections can use OAuth or
            provider-specific sign-in; other services may ask for an API key or
            basic authentication.
          </p>
          <p className="mt-3">
            A connection does not give agents access on its own. An admin must
            grant that exact instance to the agent&apos;s team. The same
            provider can have multiple independently named instances.
          </p>
        </DocSection>

        <DocSection title="Connections expose generated MCP tools">
          <p>
            Once a connection is granted to a team, its generated MCP server is
            available to that team&apos;s agents. Provider operations appear as
            tools directly.
          </p>
          <p className="mt-3">
            Revoke the connection grant to remove its generated MCP access.
            Team-owned and org-owned skills remain independent resources.
          </p>
          <DocLink href="/docs/connections-and-skills/connect-tools">
            Connect and configure a tool
          </DocLink>
        </DocSection>

        <DocsMediaPlaceholder
          type="image"
          title="Connection grant and generated tools"
          description="Use team settings with one concrete connection instance enabled and its generated MCP tools visible at a readable size."
        />

        <DocSection title="Teams decide which agents receive a connection">
          <p>
            Creating a connection adds an instance to the workspace, not every
            team. An owner or admin opens team settings and switches that
            instance on; every agent on that team inherits the grant.
          </p>
          <p className="mt-3">
            Switch the grant off and the team&apos;s agents lose the connection.
            Nothing changes for teams with their own grant.
          </p>
          <DocLink href="/docs/workspace-and-access/teams">
            Learn how teams control access
          </DocLink>
        </DocSection>

        <DocSection title="Skills are first-class resources">
          <p>
            Team members can create, edit, and archive skills owned by any team
            they belong to. Those skills are available to agents on that team
            without a connection.
          </p>
          <p className="mt-3">
            Workspace admins can also create org-wide skills and grant them to
            one or more teams.
          </p>
          <DocLink href="/docs/agents/configure-an-agent">
            Configure an agent
          </DocLink>
        </DocSection>

        <DocSection title="Know who can change access">
          <dl className="divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
            {ACCESS_ROLES.map(([role, responsibility]) => (
              <div
                key={role}
                className="grid gap-1 px-4 py-4 sm:grid-cols-[11rem_1fr] sm:gap-5"
              >
                <dt className="text-sm font-semibold text-foreground">
                  {role}
                </dt>
                <dd className="text-sm leading-6 text-muted">
                  {responsibility}
                </dd>
              </div>
            ))}
          </dl>
        </DocSection>

        <section
          aria-labelledby="change-the-smallest-boundary"
          className="rounded-xl border border-border bg-surface-secondary p-6"
        >
          <div className="flex items-start gap-3">
            <span className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-border bg-surface text-muted">
              <AppIcon icon="shield-check" className="h-4 w-4" />
            </span>
            <div>
              <h2
                id="change-the-smallest-boundary"
                className="text-lg font-semibold tracking-tight text-foreground"
              >
                Change the smallest boundary that solves the problem
              </h2>
              <p className="mt-2 max-w-xl text-sm leading-6 text-muted">
                Revoke a team grant when that team should lose a connection.
                Disconnect an account only after every team has finished with
                that concrete instance. Manage independent team and org skills
                from Settings → Skills.
              </p>
            </div>
          </div>
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
