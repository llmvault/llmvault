import type { ReactNode } from "react"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

const CADENCES = [
  ["Every hour", "Set the minute past the hour."],
  ["Every day", "Set one local time each day."],
  ["Every week", "Pick the weekdays and a local time."],
  [
    "Custom interval",
    "Repeat after a rolling number of minutes, hours, or days.",
  ],
  ["Custom cron", "Enter a five-field cron expression."],
]

export function Schedules() {
  return (
    <div className="mt-10 text-base leading-7">
      <p className="max-w-2xl text-muted">
        A schedule gives an agent the same task at a recurring time. Each run
        opens a fresh session for the team, which suits work such
        as a weekly report, a nightly data check, an hourly inbox review, or a
        monthly audit.
      </p>

      <DocsMediaPlaceholder
        type="video"
        title="Create a recurring agent schedule"
        description="Use Automations > Schedules > Add schedule to choose a team and agent, set a weekly local time, write the task, and save. Open the first session once it runs."
        className="mt-12"
      />

      <section aria-labelledby="create-a-schedule" className="mt-16">
        <h2
          id="create-a-schedule"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          Create a schedule around the result
        </h2>
        <ol className="mt-8 divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
          <Step number="1" title="Name it for the output">
            Give teammates a useful label in the Automations list, such as
            Weekly pipeline summary.
          </Step>
          <Step number="2" title="Choose a team and agent">
            The team sets the access boundary, so Hivy offers its agents only.
          </Step>
          <Step number="3" title="Set the cadence">
            Pick a recurring option or enter a custom five-field cron
            expression.
          </Step>
          <Step number="4" title="Write the recurring task">
            Tell the agent what to produce, where it should read and write, and
            how it can tell the task is done.
          </Step>
        </ol>
      </section>

      <DocsMediaPlaceholder
        type="image"
        title="Schedule fields and cadence preview"
        description="The Add schedule frame needs a readable name, team, agent, Repeat controls, timezone conversion preview, and task instructions."
        className="mt-12"
      />

      <div className="mt-16 space-y-14 border-t border-border pt-14">
        <DocSection title="Pick the cadence that matches the need">
          <dl className="divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
            {CADENCES.map(([name, detail]) => (
              <div
                key={name}
                className="grid gap-1 px-4 py-4 sm:grid-cols-[10rem_1fr] sm:gap-5"
              >
                <dt className="text-sm font-semibold text-foreground">
                  {name}
                </dt>
                <dd className="text-sm leading-6 text-muted">{detail}</dd>
              </div>
            ))}
          </dl>
          <p className="mt-4">
            You enter daily and weekly times in your browser&apos;s local
            timezone; Hivy converts them to UTC and previews the stored schedule
            before you save. A custom interval starts at creation time and rolls
            forward from there.
          </p>
        </DocSection>

        <DocSection title="Make the task safe to repeat">
          <p>
            Nobody will be there to clarify a scheduled request, so name the
            source and date range, describe the output and its destination, and
            say what to do when nothing has changed. If a repeated run could
            create duplicates, tell the agent how to avoid them.
          </p>
          <SchedulePrompt />
        </DocSection>

        <DocSection title="Review the next and previous run">
          <p>
            Open a schedule to check its status, next run time, and latest run.
            When Hivy has a session for the latest run, choose{" "}
            <strong className="text-foreground">View session</strong>
            to read the result or continue the work.
          </p>
        </DocSection>

        <DocSection title="Pause before you delete">
          <p>
            Pause a schedule when the work needs to stop for a while; Hivy keeps
            its name, team, agent, cadence, and task. Resume it later or
            delete it once the recurring task has ended.
          </p>
          <p className="mt-3">
            You can&apos;t change the agent on an existing schedule. Create
            another schedule for a different agent. Workspace owners and admins
            can edit, pause, resume, or delete schedules.
          </p>
        </DocSection>
      </div>
    </div>
  )
}

function Step({
  number,
  title,
  children,
}: {
  number: string
  title: string
  children: ReactNode
}) {
  return (
    <li className="grid gap-4 px-5 py-5 sm:grid-cols-[2rem_1fr]">
      <span className="flex h-7 w-7 items-center justify-center rounded-full bg-default text-xs font-semibold text-foreground">
        {number}
      </span>
      <div>
        <h3 className="font-semibold text-foreground">{title}</h3>
        <p className="mt-1 text-sm leading-6 text-muted">{children}</p>
      </div>
    </li>
  )
}

function SchedulePrompt() {
  return (
    <blockquote className="mt-6 rounded-xl border border-border bg-surface px-5 py-4 text-sm leading-6 text-foreground">
      Every Monday, summarize opportunities created during the previous seven
      days. Group them by owner, call out deals over $20,000, and post a short
      executive summary in this session. If the previous seven days contain no
      new opportunities, say so without creating a report.
    </blockquote>
  )
}

function DocSection({
  title,
  children,
}: {
  title: string
  children: ReactNode
}) {
  const id = "schedules-" + title.toLowerCase().replaceAll(" ", "-")
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
