import type { ReactNode } from "react"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

const TRIGGER_EXAMPLES = [
  [
    "Slack reaction",
    "Run an agent when someone adds the chosen emoji in a selected Slack channel.",
  ],
  [
    "GitHub mention",
    "Run an agent when someone mentions Hivy on an issue or pull request in a selected repository.",
  ],
  [
    "GitHub code review",
    "Start a review after someone mentions the review agent or opens a pull request.",
  ],
]

export function EventTriggers() {
  return (
    <div className="mt-10 text-base leading-7">
      <p className="max-w-2xl text-muted">
        A Slack reaction or GitHub event can hand work to an agent as soon as it
        happens. Set the account, event source, team, agent, and instructions
        once; each match starts a new session.
      </p>

      <DocsMediaPlaceholder
        type="video"
        title="Install a connected-app trigger"
        description="Keep Automations > Install trigger readable while you configure either a Slack reaction or GitHub event. After sending one matching event, open the session it starts."
        className="mt-12"
      />

      <section aria-labelledby="available-event-triggers" className="mt-16">
        <h2
          id="available-event-triggers"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          Start from a supported event
        </h2>
        <p className="mt-3 max-w-2xl text-muted">
          The trigger catalog lists the event templates Hivy can install today.
          A template asks only for settings that affect its event.
        </p>
        <dl className="mt-8 divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
          {TRIGGER_EXAMPLES.map(([title, detail]) => (
            <div
              key={title}
              className="grid gap-1 px-5 py-4 sm:grid-cols-[11rem_1fr] sm:gap-5"
            >
              <dt className="font-semibold text-foreground">{title}</dt>
              <dd className="text-sm leading-6 text-muted">{detail}</dd>
            </div>
          ))}
        </dl>
      </section>

      <DocsMediaPlaceholder
        type="image"
        title="A complete event trigger configuration"
        description="Frame the entire trigger form at a readable size: connection, external resource, team, agent, instructions, and the event value when the form has one."
        className="mt-12"
      />

      <div className="mt-16 space-y-14 border-t border-border pt-14">
        <DocSection title="Connect the account that receives the event">
          <p>
            Pick an active provider connection and the resource Hivy should
            watch. Slack asks for a workspace and channel; GitHub asks for a
            connection and repository.
          </p>
          <p className="mt-3">
            Hivy won&apos;t install the trigger unless that connection still
            works and the chosen agent can use the provider connection through a
            valid connection.
          </p>
        </DocSection>

        <DocSection title="Choose where the agent works">
          <p>
            GitHub triggers ask for a team, which determines the available
            agents. A Slack reaction trigger uses its selected Slack channel as
            the event source, then creates or reuses the matching session.
          </p>
          <p className="mt-3">
            A Slack reaction trigger also needs the emoji that counts as its
            signal. Hivy blocks a duplicate with the same agent, source, event,
            and value.
          </p>
        </DocSection>

        <DocSection title="Write instructions for the event, not the example">
          <p>
            Tell the agent what every matching event should produce. The event
            brings its repository, message, issue or pull request, sender, and
            other provider data; the instructions hold the outcome and rules
            that stay fixed between runs.
          </p>
        </DocSection>

        <DocSection title="Disable a trigger without losing its setup">
          <p>
            Open an installed trigger to change its name, resource, team,
            agent, event settings, or instructions. Turn its status off to
            ignore matches without losing the setup; delete it when you
            won&apos;t use it again.
          </p>
          <p className="mt-3">
            Workspace owners and admins can edit, disable, or delete an
            installed trigger. Other members see only triggers attached to
            agents and teams they can access.
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
  const id = "event-triggers-" + title.toLowerCase().replaceAll(" ", "-")
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
