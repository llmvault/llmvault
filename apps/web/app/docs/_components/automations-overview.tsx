import Link from "next/link"
import type { ReactNode } from "react"
import { AppIcon } from "@/components/icon"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

const START_OPTIONS = [
  {
    title: "A connected app event",
    detail:
      "Use a Slack reaction or GitHub mention to start work when the event happens.",
    href: "/docs/automations/event-triggers",
  },
  {
    title: "A recurring schedule",
    detail:
      "Ask an agent to run hourly, daily, weekly, on an interval, or from a cron expression.",
    href: "/docs/automations/schedules",
  },
  {
    title: "An HTTP request",
    detail:
      "Have another system send data to a protected webhook URL and start an agent.",
    href: "/docs/automations/http-webhooks",
  },
]

export function AutomationsOverview() {
  return (
    <div className="mt-10 text-base leading-7">
      <p className="max-w-2xl text-muted">
        An automation starts agent work without waiting for someone to open
        Hivy. It still runs a named agent inside a channel, where the
        responsible team can see the request, result, follow-ups, and cost.
      </p>

      <DocsMediaPlaceholder
        type="video"
        title="Turn a repeated task into an automation"
        description="Start in Automations and create a connected-app trigger, a schedule, and a webhook; finish on the channel session from one run. The form controls and session text need to remain readable."
        className="mt-12"
      />

      <section aria-labelledby="choose-how-work-starts" className="mt-16">
        <h2
          id="choose-how-work-starts"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          Choose how the work starts
        </h2>
        <p className="mt-3 max-w-2xl text-muted">
          Begin with the signal your system already produces; Hivy turns it into
          a regular agent session instead of sending the work to a separate
          automation workspace.
        </p>
        <div className="mt-8 divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
          {START_OPTIONS.map((option) => (
            <Link
              key={option.title}
              href={option.href}
              className="group flex items-start justify-between gap-5 px-5 py-5 transition-colors hover:bg-default focus-visible:bg-default focus-visible:outline-none"
            >
              <span>
                <span className="font-semibold text-foreground">
                  {option.title}
                </span>
                <span className="mt-1 block text-sm leading-6 text-muted">
                  {option.detail}
                </span>
              </span>
              <AppIcon
                icon="arrow-right"
                className="mt-1 h-4 w-4 shrink-0 text-muted transition-transform group-hover:translate-x-0.5 group-hover:text-foreground"
              />
            </Link>
          ))}
        </div>
      </section>

      <div className="mt-16 space-y-14 border-t border-border pt-14">
        <DocSection title="Scope every run before you automate it">
          <p>
            The channel puts every new session beside related team work and
            limits the agent picker to that channel&apos;s team. Automated and
            manual sessions therefore follow the same access boundary.
          </p>
          <p className="mt-3">
            Choose an agent prepared for the job and describe the finished
            result in its instructions. Event triggers add connected-app data or
            an HTTP payload; a schedule sends the saved task each time it runs.
          </p>
        </DocSection>

        <DocSection title="Manage automations in one place">
          <p>
            Open <strong className="text-foreground">Automations</strong>, then
            switch between Connections, Schedules, and Webhooks. Each list has
            search and filters; open an item to check its scope or status.
          </p>
          <p className="mt-3">
            A team member can create an automation for a team they manage.
            Workspace owners and admins control existing automations: they can
            edit them, pause or disable them, or delete them.
          </p>
        </DocSection>

        <DocSection title="Each run becomes a session">
          <p>
            When an event arrives or a scheduled time comes due, Hivy creates a
            session in the chosen channel and gives the task to the agent. Open
            the session to check its work and cost; if the result needs a
            change, ask there.
          </p>
          <DocLink href="/docs/automations/runs-and-troubleshooting">
            Review automation runs
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
  const id = "automations-" + title.toLowerCase().replaceAll(" ", "-")
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
