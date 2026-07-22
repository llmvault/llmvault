import Link from "next/link"
import type { ReactNode } from "react"
import { AppIcon } from "@/components/icon"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

const TEAM_SCOPE = [
  {
    number: "01",
    title: "People",
    description:
      "A member sees work from their teams, not every team in the workspace.",
  },
  {
    number: "02",
    title: "Agents and sessions",
    description: "Agents and their sessions stay within one team.",
  },
  {
    number: "03",
    title: "Shared resources",
    description:
      "Connections, skills, knowledge sources, and environment variables are granted or saved at the team boundary.",
  },
  {
    number: "04",
    title: "External routing",
    description:
      "A connected provider resource can route new conversations to one agent in the team.",
  },
]

export function Teams() {
  return (
    <div className="mt-10 text-base leading-7">
      <p className="max-w-2xl text-muted">
        Teams separate one group&apos;s work from another. Give Sales or Support
        its own people, agents, connections, skills, knowledge, and secrets
        without opening the rest of the workspace.
      </p>

      <section aria-labelledby="what-a-team-controls" className="mt-14">
        <h2
          id="what-a-team-controls"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          What a team controls
        </h2>
        <p className="mt-3 max-w-2xl text-muted">
          A team draws the boundary around daily work: membership controls who
          gets in, while team settings control what its agents can use.
        </p>

        <ol className="mt-8 divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
          {TEAM_SCOPE.map((item) => (
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
        type="image"
        title="Team details and resource access"
        description="Take this screenshot from the team details page at 4K and 100% browser zoom. Include members, environment variables, external routing, and granted resources; use demo values and hide every email address and secret."
        className="mt-12"
      />

      <div className="mt-16 space-y-14 border-t border-border pt-14">
        <DocSection title="Create a team for durable ownership">
          <p>
            Workspace owners and admins create teams under{" "}
            <strong>Settings</strong> &gt; <strong>Teams</strong> &gt;{" "}
            <strong>New team</strong>. Use the name people already use for the
            group, such as Sales or Engineering.
          </p>
          <p className="mt-3">
            Hivy adds the default Hivy agent and puts the creator on the team.
            There&apos;s nothing else to prepare before the first session.
          </p>
          <DocLink href="/w/settings/teams">Open team settings</DocLink>
        </DocSection>

        <DocSection title="Add the people who own the work">
          <p>
            Add a current workspace member from the team page. For someone new,
            choose <strong>Invite member</strong>, enter their email, then set
            their workspace role and team membership before sending the invite.
          </p>
          <p className="mt-3">
            Once the person accepts, Hivy puts them on the selected teams. Their
            team membership can change later without changing their workspace
            role.
          </p>
          <DocLink href="/docs/workspace-and-access/roles-and-invitations">
            Learn about access control
          </DocLink>
        </DocSection>

        <DocSection title="Grant shared resources once">
          <p>
            Owners and admins grant workspace connections, skills, and knowledge
            sources on the team page. Team-owned skills appear automatically;
            workspace skills require an explicit team grant.
          </p>
          <p className="mt-3">
            Connections become available to every agent on the team, but one
            agent may switch off an optional connection. Catalog requirements
            stay on for the agent that requires them.
          </p>
          <DocLink href="/docs/knowledge-and-memory/indexing-and-access">
            Manage knowledge access
          </DocLink>
        </DocSection>

        <DocSection title="Add secrets and external routes deliberately">
          <p>
            Team environment variables are encrypted and write-only after you
            save them. Sessions for that team receive the values, while agents
            see only each variable&apos;s name and description in their context.
          </p>
          <p className="mt-3">
            External routing assigns a provider resource, such as a Slack
            channel, to one team agent. A conversation already tied to an agent
            keeps that affinity even after the route changes.
          </p>
          <DocLink href="/docs/administration/workspace-settings">
            Configure context and environments
          </DocLink>
        </DocSection>

        <DocSection title="Give the team agents for its jobs">
          <p>
            Build agents for the jobs a team handles, then run them in any of
            that team&apos;s sessions. Every agent belongs to one team, so Hivy
            won&apos;t let an agent from another team run its work.
          </p>
          <DocLink href="/docs/agents/configure-an-agent">
            Create and configure an agent
          </DocLink>
        </DocSection>

        <DocsMediaPlaceholder
          type="video"
          title="Create a team and set its access"
          description="Record a 60 to 90 second walkthrough at 4K and 100% browser zoom. Create a team, add one member, grant a connection and skill, add a masked demo environment variable, and route one provider resource; hide email addresses and real secrets."
          bleed={false}
        />

        <section
          aria-labelledby="team-structure-guidance"
          className="rounded-xl border border-border bg-surface-secondary p-6"
        >
          <div className="flex items-start gap-3">
            <span className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-border bg-surface text-muted">
              <AppIcon icon="info" className="h-4 w-4" />
            </span>
            <div>
              <h2
                id="team-structure-guidance"
                className="text-lg font-semibold tracking-tight text-foreground"
              >
                Keep team boundaries stable
              </h2>
              <p className="mt-2 max-w-xl text-sm leading-6 text-muted">
                Make teams match groups that will still own the work next
                quarter. Projects and customer accounts usually belong in
                sessions, since a new team for every task makes access harder to
                follow.
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
