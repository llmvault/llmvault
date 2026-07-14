import Link from "next/link"
import type { ReactNode } from "react"
import { AppIcon } from "@/components/icon"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

const CHANNEL_CHOICES = [
  {
    title: "Team",
    description:
      "The selected team controls who gets in and which agents appear in the picker.",
    icon: "users" as const,
  },
  {
    title: "Category",
    description:
      "The category tells Hivy what this channel should remember about its work.",
    icon: "brain" as const,
  },
  {
    title: "Default agent",
    description:
      "New work starts with this agent unless someone picks another team agent.",
    icon: "bot" as const,
  },
]

export function Channels() {
  return (
    <div className="mt-10 text-base leading-7">
      <p className="max-w-2xl text-muted">
        A channel keeps related sessions together. Put each task in its own
        session, and the team gets one place to find the request, result, cost,
        and any artifacts later.
      </p>

      <section aria-labelledby="what-a-channel-controls" className="mt-14">
        <h2
          id="what-a-channel-controls"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          Set the scope before work starts
        </h2>
        <p className="mt-3 max-w-2xl text-muted">
          Before anyone starts a session, choose the team, category, and default
          agent. Those settings control access, memory, and the agent Hivy picks
          first.
        </p>

        <ul className="mt-8 divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
          {CHANNEL_CHOICES.map((choice) => (
            <li key={choice.title} className="p-5">
              <AppIcon icon={choice.icon} className="h-4 w-4 text-accent" />
              <h3 className="mt-5 text-base font-semibold text-foreground">
                {choice.title}
              </h3>
              <p className="mt-2 text-sm leading-6 text-muted">
                {choice.description}
              </p>
            </li>
          ))}
        </ul>
      </section>

      <DocsMediaPlaceholder
        type="image"
        title="Create channel form"
        description="Take this screenshot after selecting a team in the create channel form. At 4K and 100% browser zoom, include Name, Team, Category, Agent, and the optional Slack section; use a realistic channel name and remove browser chrome."
        className="mt-12"
      />

      <div className="mt-16 space-y-14 border-t border-border pt-14">
        <DocStep number="1" title="Create a home for repeat work">
          <p>
            Make a channel when the same kind of work will come back, such as
            customer support or launch planning. Each task gets a separate
            session inside it.
          </p>
          <p className="mt-3">
            Select <strong>New channel</strong>, then use a short name that the
            team will spot quickly in the sidebar.
          </p>
        </DocStep>

        <DocStep number="2" title="Choose the team and category">
          <p>
            Pick the team that owns the work. Hivy lists only that team&apos;s
            agents, while active team members can open its standard channels.
          </p>
          <p className="mt-3">
            Next, match the category to the work. Hivy uses it to decide which
            details belong in long-term memory; <strong>Engineering</strong>,
            for example, favors technical decisions and incidents. Pick{" "}
            <strong>General</strong> when the channel doesn&apos;t need a
            specific memory mission.
          </p>
        </DocStep>

        <DocStep number="3" title="Pick a default agent">
          <p>
            Hivy lists agents from the chosen team. Set the one that should
            handle new work by default; someone can still pick a different team
            agent when a session needs another specialist.
          </p>
          <p className="mt-3">
            Missing the agent you need? Add it to the team, then return to the
            channel form.
          </p>
        </DocStep>

        <DocStep number="4" title="Start the first session">
          <p>
            Open the channel and give the agent a specific task. Hivy lists
            later sessions underneath it, together with generated files, sheets,
            apps, and the cost of each run.
          </p>
          <p className="mt-3">
            Learn how to{" "}
            <DocLink href="/docs/agents/sessions">run agent sessions</DocLink>{" "}
            or pick up an earlier result.
          </p>
        </DocStep>

        <DocsMediaPlaceholder
          type="video"
          title="Create a channel and run its first session"
          description="Record a 60 second walkthrough at 4K and 100% browser zoom. Create an Engineering channel, explain why you chose its team and agent, submit one short task, then show the new session in the sidebar."
          bleed={false}
        />

        <DocSection title="Public and private channel access">
          <p>
            Active team members can open a public channel. A private channel
            appears only to its explicit members, although workspace owners and
            admins can still manage it when they need to fix access or setup.
          </p>
          <p className="mt-3">
            See{" "}
            <DocLink href="/docs/workspace-and-access/roles-and-invitations">
              access control
            </DocLink>{" "}
            for the role rules.
          </p>
        </DocSection>

        <DocSection title="Bring an existing Slack channel into Hivy">
          <p>
            You can link Slack while creating the channel. The Slack thread and
            Hivy session then stay connected, but the Hivy team still controls
            access, memory, and agent selection.
          </p>
          <p className="mt-3">
            Follow the{" "}
            <DocLink href="/docs/workspace-and-access/slack-channels">
              Slack channel guide
            </DocLink>{" "}
            before setting it up.
          </p>
        </DocSection>
      </div>
    </div>
  )
}

function DocStep({
  number,
  title,
  children,
}: {
  number: string
  title: string
  children: ReactNode
}) {
  const id =
    "channel-step-" + number + "-" + title.toLowerCase().replaceAll(" ", "-")

  return (
    <section aria-labelledby={id}>
      <p className="text-xs font-semibold tracking-[0.12em] text-muted uppercase">
        Step {number}
      </p>
      <h2
        id={id}
        className="mt-2 text-xl font-semibold tracking-tight text-foreground"
      >
        {title}
      </h2>
      <div className="mt-3 max-w-2xl text-muted">{children}</div>
    </section>
  )
}

function DocSection({
  title,
  children,
}: {
  title: string
  children: ReactNode
}) {
  const id = "channel-" + title.toLowerCase().replaceAll(" ", "-")

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
