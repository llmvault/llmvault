import type { ReactNode } from "react"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

const START_CHOICES = [
  {
    label: "Team",
    detail: "Limits the agents and resources you can choose for this task.",
  },
  {
    label: "Agent",
    detail:
      "Selects the specialist, instructions, and tools used in the session.",
  },
  {
    label: "Model",
    detail: "Sets the model and cost profile for this session only.",
  },
  {
    label: "Reasoning effort",
    detail:
      "Controls how much reasoning the selected model uses before it answers.",
  },
]

export function AgentSessions() {
  return (
    <div className="mt-10 text-base leading-7">
      <p className="max-w-2xl text-muted">
        A session holds one task with one agent in a team. The request,
        follow-ups, generated work, and cost stay together, so someone can
        reopen the task without reconstructing what happened.
      </p>

      <DocsMediaPlaceholder
        type="video"
        title="Run and refine an agent session"
        description="Record New chat, the scope and model choices, the agent’s progress, and one follow-up request on the result."
        className="mt-12"
      />

      <section aria-labelledby="start-a-session" className="mt-16">
        <h2
          id="start-a-session"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          Start with the right scope
        </h2>
        <p className="mt-3 max-w-2xl text-muted">
          Open <strong className="text-foreground">New chat</strong>. Before you
          send anything, check the four selectors in the composer because they
          determine where the session lives and how it runs.
        </p>
        <dl className="mt-8 divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
          {START_CHOICES.map((choice) => (
            <div key={choice.label} className="p-5">
              <dt className="font-semibold text-foreground">{choice.label}</dt>
              <dd className="mt-2 text-sm leading-6 text-muted">
                {choice.detail}
              </dd>
            </div>
          ))}
        </dl>
      </section>

      <DocsMediaPlaceholder
        type="image"
        title="The New chat composer with session choices"
        description="Show the team, agent, model, reasoning effort, attachment, voice, and send controls at readable scale."
        className="mt-12"
      />

      <div className="mt-16 space-y-14 border-t border-border pt-14">
        <DocSection title="Choose the agent before the model">
          <p>
            Pick the specialist whose job matches the request, since that choice
            brings its instructions, tools, and team access into the session.
            After that, choose the lowest-cost model you trust to finish the
            work.
          </p>
          <p className="mt-3">
            Hivy preselects the agent&apos;s default model. Change it when this
            specific task needs stronger reasoning or code work, and set the
            reasoning effort before sending the first message.
          </p>
        </DocSection>

        <DocSection title="Give the agent a finish line">
          <p>
            Name the deliverable and its audience, then include the source
            material and limits that affect the answer. Vague requests make the
            agent guess; a finish line gives you something concrete to review.
          </p>
          <PromptExample />
          <p className="mt-4">
            Attach images when the task depends on visual details. Voice input
            works better for requests that are faster to explain aloud.
          </p>
        </DocSection>

        <DocSection title="Follow the work without reading every tool call">
          <p>
            Hivy groups tool activity under the working section, which stays
            collapsed when you only need the answer. Expand it to inspect the
            searches, commands, file changes, plans, or delegated tasks behind
            that answer.
          </p>
          <p className="mt-3">
            If the agent heads in the wrong direction, stop the active turn.
            Hivy keeps the session intact for a corrected follow-up.
          </p>
          <p className="mt-3">
            An agent may pause with one to three structured questions. Choose an
            option, add a free-form answer when needed, then submit the answers
            so the same turn can continue.
          </p>
        </DocSection>

        <DocSection title="Continue from the existing result">
          <p>
            Keep revisions in the same session when they depend on an existing
            result. The agent can use the history and working environment rather
            than rebuild the output.
          </p>
          <p className="mt-3">
            A different goal belongs in a new session, where unrelated history
            can&apos;t steer the answer and the session list stays easier to
            scan.
          </p>
        </DocSection>

        <DocSection title="Review cost as the session grows">
          <p>
            The composer shows running credits and estimated dollar cost. Check
            it after real tasks, then move routine work to a faster model when a
            stronger one doesn&apos;t improve the result enough to justify its
            price.
          </p>
        </DocSection>

        <DocSection title="Share, rename, or archive the session">
          <p>
            A specific name helps teammates find the session later. Add
            workspace members as participants when they need access, and archive
            finished work that no longer belongs in the active session list.
          </p>
          <p className="mt-3">
            To bring it back, open{" "}
            <strong className="text-foreground">Settings</strong>, choose{" "}
            <strong className="text-foreground">Archived chats</strong>, and
            select <strong className="text-foreground">Restore</strong>.
          </p>
        </DocSection>

        <section
          aria-labelledby="session-boundary"
          className="rounded-xl border border-border bg-surface-secondary p-6"
        >
          <h2
            id="session-boundary"
            className="text-lg font-semibold tracking-tight text-foreground"
          >
            Sessions inherit the team boundary
          </h2>
          <p className="mt-2 max-w-xl text-sm leading-6 text-muted">
            A session can use only its team&apos;s agents. Sharing or reopening
            a session doesn&apos;t bypass the team boundary or participant
            access.
          </p>
        </section>
      </div>
    </div>
  )
}

function PromptExample() {
  return (
    <blockquote className="mt-6 rounded-xl border border-border bg-surface px-5 py-4 text-sm leading-6 text-foreground">
      Build a Hivy Sheet comparing five competitors for the sales team. Include
      pricing, target customer, main differentiator, and a short note on the two
      strongest opportunities for us.
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
  const id = "sessions-" + title.toLowerCase().replaceAll(" ", "-")
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
