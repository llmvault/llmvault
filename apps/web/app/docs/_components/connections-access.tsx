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
    title: "Plugin",
    description:
      "An agent gets service skills through a plugin, which also records the connection or database those skills need.",
  },
  {
    number: "03",
    title: "Team access",
    description:
      "An admin grants the installed plugin to one team, making it available to that team's agents.",
  },
  {
    number: "04",
    title: "Agent access",
    description:
      "Agents inherit their team's plugins. You can turn off an optional one for a single agent without changing the rest of the team.",
  },
]

const ACCESS_ROLES = [
  [
    "Workspace owners and admins",
    "They handle external accounts, plugin installation, connection resources, and team grants.",
  ],
  [
    "Team members",
    "They can inspect the plugins granted to their teams and use agents that have those plugins.",
  ],
  [
    "Members managing an agent",
    "A member who manages an agent may turn off an optional inherited plugin. Hivy won't let them switch off required or always-on plugins.",
  ],
]

export function ConnectionsAccess() {
  return (
    <div className="mt-10 text-base leading-7">
      <p className="max-w-2xl text-muted">
        Connect the company account once; plugins map it to a job, and team
        grants decide which agents may use it. You don&apos;t need a separate
        sign-in for each agent.
      </p>

      <section aria-labelledby="four-layers-of-access" className="mt-14">
        <h2
          id="four-layers-of-access"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          Follow the four layers of access
        </h2>
        <p className="mt-3 max-w-2xl text-muted">
          A connection alone doesn&apos;t open an external service to an agent.
          Hivy checks all four layers whenever the agent calls a plugin skill.
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
        description="Record an admin opening a plugin and checking its connection. Keep the team grant visible as the agent inherits the plugin, then turn off an optional plugin for that agent."
      />

      <div className="mt-16 space-y-14 border-t border-border pt-14">
        <DocSection title="A connection is workspace-level">
          <p>
            Hivy calls the approved link to an external account a connection.
            Slack, GitHub, Notion, and Google Drive connections can use OAuth or
            provider-specific sign-in; other services may ask for an API key or
            basic authentication.
          </p>
          <p className="mt-3">
            That connection doesn&apos;t give agents access on its own. An admin
            must install a matching plugin in the workspace, and the
            agent&apos;s team must have a grant for it.
          </p>
        </DocSection>

        <DocSection title="Plugins define what an agent can do">
          <p>
            Plugins contain the skills for a job. Their requirements name the
            external service or database behind those skills, and Hivy
            won&apos;t let an admin add a plugin until every required connection
            works.
          </p>
          <p className="mt-3">
            A GitHub plugin, for example, may need a repository selection. Hivy
            saves that choice as the default boundary for the connection; the
            account stays connected, but agents see only the repositories you
            picked.
          </p>
          <DocLink href="/docs/plugins-and-connections/connect-tools">
            Connect and configure a tool
          </DocLink>
        </DocSection>

        <DocsMediaPlaceholder
          type="image"
          title="Plugin requirements and access state"
          description="Use a plugin detail page with an active connection. The frame must show Required connections, Resources, install status, and the plugin's skills at a readable size."
        />

        <DocSection title="Teams decide which agents receive a plugin">
          <p>
            Adding a plugin puts it in the workspace catalog, not on every team.
            An owner or admin opens the team&apos;s settings and switches the
            plugin on; every agent on that team inherits the grant.
          </p>
          <p className="mt-3">
            Switch the grant off and the team&apos;s agents lose the plugin.
            Nothing changes for teams with their own grant.
          </p>
          <DocLink href="/docs/workspace-and-access/teams">
            Learn how teams control access
          </DocLink>
        </DocSection>

        <DocSection title="Narrow one agent without changing its team">
          <p>
            Open an installed agent to check what it inherited. Turning off an
            optional team plugin there affects that agent alone, so the
            team&apos;s other agents keep using it.
          </p>
          <p className="mt-3">
            Hivy locks plugins that the agent requires, always-on plugins, and
            plugins supplied to the team&apos;s default agent.
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
                Turn off an optional plugin on one agent when only that agent
                needs a smaller toolset. If the whole team should lose it,
                remove the team grant; remove the workspace plugin or disconnect
                the account only after every team has finished with it.
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
