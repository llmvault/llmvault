import Link from "next/link"
import type { ReactNode } from "react"
import { AppIcon } from "@/components/icon"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

const SETUP_STEPS = [
  {
    number: "01",
    title: "Create your first team",
    description:
      "Name the group that will own the first agents, connections, knowledge, and sessions.",
  },
  {
    number: "02",
    title: "Connect one tool",
    description:
      "Choose a provider and finish its sign-in. Hivy grants the new connection to the first team during setup.",
  },
  {
    number: "03",
    title: "Start the first chat",
    description:
      "Finish the short welcome screen, then open a conversation with the team's Hivy agent.",
  },
]

export function SetUpWorkspace() {
  return (
    <div className="mt-10 text-base leading-7">
      <p className="max-w-2xl text-muted">
        New workspaces begin with a guided setup. Create the first team and
        connect one tool; Hivy prepares the team&apos;s default agent and sends
        you to the first chat.
      </p>

      <section aria-labelledby="workspace-setup-flow" className="mt-14">
        <h2
          id="workspace-setup-flow"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          Finish setup in three steps
        </h2>
        <ol className="mt-8 divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
          {SETUP_STEPS.map((step) => (
            <li key={step.title} className="p-5">
              <span className="text-xs font-semibold tracking-[0.12em] text-muted uppercase">
                {step.number}
              </span>
              <h3 className="mt-4 font-semibold text-foreground">
                {step.title}
              </h3>
              <p className="mt-2 text-sm leading-6 text-muted">
                {step.description}
              </p>
            </li>
          ))}
        </ol>
      </section>

      <DocsMediaPlaceholder
        type="video"
        title="Set up a new Hivy workspace"
        description="Record the complete first-run flow with demo data: name the first team, connect one provider, view the setup-complete screen, and start the first chat. Keep account credentials out of the recording."
        className="mt-12"
      />

      <div className="mt-16 space-y-14 border-t border-border pt-14">
        <DocSection title="Create the team that owns the first work">
          <p>
            A team is the access boundary for its agents and sessions. Use a
            clear name for the people or job that will share the first
            connection. You can add more teams later from Settings.
          </p>
          <p className="mt-3">
            Hivy creates a default Hivy agent for the team. It cannot be
            deleted, and it gives the team a ready starting point before you
            install or create specialists.
          </p>
          <DocLink href="/docs/workspace-and-access/teams">
            Learn how teams control access
          </DocLink>
        </DocSection>

        <DocsMediaPlaceholder
          type="image"
          title="Create your first team"
          description="Capture the first setup step with a short fictional team name in the Team name field and the setup progress visible."
        />

        <DocSection title="Connect one account to continue">
          <p>
            Search the connection catalog and choose the account agents should
            use. OAuth providers open their own sign-in; other providers may ask
            for an API key or account details inside Hivy.
          </p>
          <p className="mt-3">
            Setup requires at least one connected account. The first connection
            is granted to the team you just created, so its agents can use the
            generated tools immediately. You can add another instance or narrow
            agent access after setup.
          </p>
          <DocLink href="/docs/connections-and-skills/connect-tools">
            Connect and configure tools
          </DocLink>
        </DocSection>

        <DocSection title="Use the welcome credits on a real task">
          <p>
            A new workspace starts with 1,000 credits and does not need a card.
            After the setup-complete screen, choose{" "}
            <strong className="text-foreground">Start your first chat</strong>
            and give Hivy a small task with a result you can check.
          </p>
          <DocLink href="/docs/run-your-first-agent">
            Run your first agent
          </DocLink>
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
  const id = "setup-" + title.toLowerCase().replaceAll(" ", "-")
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
