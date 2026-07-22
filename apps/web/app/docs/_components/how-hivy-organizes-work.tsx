import Link from "next/link"
import type { ReactNode } from "react"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

const WORK_HIERARCHY = [
  {
    title: "Workspace",
    description: "The company home for everyone and the work they share.",
  },
  {
    title: "Team",
    description: "People and agents assigned to the same area of work.",
  },
  {
    title: "Agent",
    description: "One specialist, owned by a team, with a saved job and tools.",
  },
  {
    title: "Session",
    description:
      "One agent task with its context, cost, result, and artifacts.",
  },
]

export function HowHivyOrganizesWork() {
  return (
    <div className="mt-10 text-base leading-7">
      <p className="max-w-2xl text-muted">
        Hivy stores agent work with the team that owns it. Team members can find
        the session later, and access controls decide which tools, knowledge,
        connections, and context its agents can use.
      </p>

      <section aria-labelledby="work-hierarchy" className="mt-14">
        <h2
          id="work-hierarchy"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          Where a session lives
        </h2>
        <p className="mt-3 max-w-2xl text-muted">
          Every session belongs to a team and an agent. People can find the work
          without opening the whole workspace to every member or agent.
        </p>

        <ol className="mt-8 divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
          {WORK_HIERARCHY.map((item, index) => (
            <li key={item.title} className="p-5">
              <span className="text-xs font-semibold tracking-[0.12em] text-muted uppercase">
                {String(index + 1).padStart(2, "0")}
              </span>
              <h3 className="mt-5 text-base font-semibold text-foreground">
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
        title="See where work lives"
        description="Capture the workspace at 100% zoom with one expanded team in the left sidebar, named sessions, and one session open in the main area."
      />

      <div className="mt-16 space-y-14 border-t border-border pt-14">
        <DocSection title="Agents belong to teams">
          <p>
            Each agent belongs to one team. That team can create specialists for
            any work it owns, and Hivy offers those agents only inside the team.
          </p>
          <p className="mt-3">
            Workspace and team controls set the tools, connections, knowledge,
            and context available to agents. Other company resources stay out of
            reach. Learn how to{" "}
            <DocLink href="/docs/agents/configure-an-agent">
              create and configure an agent
            </DocLink>
            .
          </p>
        </DocSection>

        <DocSection title="Teams organize related work">
          <p>
            Teams own their agents, sessions, and shared resources. Team members
            can read the request and result, then continue where someone stopped
            without searching through one person’s private history.
          </p>
          <p className="mt-3">
            Use teams to define shared access, then keep each task in its own
            session.
          </p>
        </DocSection>

        <DocSection title="Useful results outlive the session">
          <p>
            Hivy keeps generated files and artifacts with their source session.
            Agent Drive preserves useful files across one agent&apos;s sessions;
            Sheets store shared team data, and each app binds to one Sheet.
          </p>
        </DocSection>

        <DocSection title="Automations use the same structure">
          <p>
            A schedule, connected-app event, or HTTP webhook can start an agent.
            During setup, you choose the team and agent; the team will find each
            run there afterward.
          </p>
          <p className="mt-3">
            Read the{" "}
            <DocLink href="/docs/automations/overview">
              automations overview
            </DocLink>
            .
          </p>
        </DocSection>
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
      className="rounded-sm font-medium text-foreground underline decoration-border underline-offset-4 transition-colors hover:text-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus"
    >
      {children}
    </Link>
  )
}
