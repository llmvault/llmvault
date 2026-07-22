import Link from "next/link"
import type { ReactNode } from "react"
import { AppIcon } from "@/components/icon"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

const FIRST_CHECKS = [
  ["Status", "Enable the trigger or webhook, or make the schedule active."],
  [
    "Source",
    "Match the connected account, repository, Slack channel, emoji, or webhook URL to the event you sent.",
  ],
  [
    "Scope",
    "Check that the agent still belongs to the team and can use the provider connection.",
  ],
  [
    "Instructions",
    "Read the saved task as the agent would; it needs a clear result and a role for the event data.",
  ],
]

export function AutomationRuns() {
  return (
    <div className="mt-10 text-base leading-7">
      <p className="max-w-2xl text-muted">
        A successful automation handoff creates a regular Hivy session in the
        configured team. That session is the run record, with the request, agent
        work, tool calls, result, artifacts, and cost together.
      </p>

      <DocsMediaPlaceholder
        type="video"
        title="Trace an automation from signal to result"
        description="Compare all three run paths in one video: a connected-app event, a schedule, and an HTTP webhook call. Open each resulting session, expand the agent's work, check the result and cost, then point out any latest-run link."
        className="mt-12"
      />

      <section aria-labelledby="find-the-run" className="mt-16">
        <h2
          id="find-the-run"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          Find the run in its team
        </h2>
        <p className="mt-3 max-w-2xl text-muted">
          Open the team chosen during setup. Hivy places automated sessions
          beside the team&apos;s other work, where teammates with access can
          read the context or continue from the result.
        </p>
        <p className="mt-3 max-w-2xl text-muted">
          Schedule and webhook detail pages show the latest run too. Choose
          <strong className="text-foreground"> View session</strong> when Hivy
          provides the link; a schedule also reports its latest status and next
          run time.
        </p>
      </section>

      <DocsMediaPlaceholder
        type="image"
        title="An automated session and its source"
        description="Place one automated session in a readable frame with its team, source, original task or event data, result, work duration, and cost visible."
        className="mt-12"
      />

      <div className="mt-16 space-y-14 border-t border-border pt-14">
        <DocSection title="Start with four checks">
          <dl className="divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
            {FIRST_CHECKS.map(([name, detail]) => (
              <div
                key={name}
                className="grid gap-1 px-4 py-4 sm:grid-cols-[8rem_1fr] sm:gap-5"
              >
                <dt className="text-sm font-semibold text-foreground">
                  {name}
                </dt>
                <dd className="text-sm leading-6 text-muted">{detail}</dd>
              </div>
            ))}
          </dl>
        </DocSection>

        <DocSection title="If a connected event does not run">
          <p>
            Check that the provider connection still works and that the event
            matches the installed trigger. Look closely at the repository or
            Slack channel, mention type, and reaction emoji; a disabled trigger
            ignores every match.
          </p>
          <p className="mt-3">
            Hivy deduplicates provider deliveries that carry the same delivery
            identifier, so a provider retry continues the original handoff
            instead of opening another session.
          </p>
        </DocSection>

        <DocSection title="If a schedule does not run">
          <p>
            Check the schedule&apos;s status and next run time. You choose daily
            and weekly times in local time, while Hivy stores and runs them in
            UTC; a custom interval starts at creation time and may fall between
            wall-clock boundaries.
          </p>
          <p className="mt-3">
            Open the schedule for its latest status. If someone moved its agent
            to another team or archived it, create a new schedule with an agent
            that belongs to the selected team.
          </p>
        </DocSection>

        <DocSection title="If Hivy rejects an HTTP webhook">
          <ul className="space-y-2">
            <li>
              <strong className="text-foreground">401:</strong> the shared
              secret is missing or wrong, or the webhook has no configured
              secret.
            </li>
            <li>
              <strong className="text-foreground">404:</strong> check the URL.
              Hivy also returns this status after someone disables or deletes
              the webhook, or archives its agent.
            </li>
            <li>
              <strong className="text-foreground">413:</strong> trim the request
              body below 256 KB.
            </li>
          </ul>
          <p className="mt-3">
            A 200 response means Hivy accepted the request for asynchronous
            processing; check the resulting session for the agent&apos;s final
            result.
          </p>
        </DocSection>

        <DocSection title="Fix the automation or refine the session">
          <p>
            Change the automation when future runs need another source, cadence,
            team, or instruction. A schedule cannot change agents; create a new
            one when a different agent should run the task. If only one result
            is wrong, correct it with a follow-up in that session and leave the
            saved automation alone.
          </p>
          <DocLink href="/docs/agents/sessions">
            Learn how agent sessions work
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
  const id = "automation-runs-" + title.toLowerCase().replaceAll(" ", "-")
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
