import Link from "next/link"
import type { ReactNode } from "react"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

const FIRST_DECISIONS = [
  {
    number: "01",
    title: "Give it one clear job",
    description:
      "Name the result the team expects; a narrow job makes mistakes easier to spot and correct.",
  },
  {
    number: "02",
    title: "Choose the owning team",
    description:
      "Choose the team that owns the work because that choice controls the agent’s channels and plugins.",
  },
  {
    number: "03",
    title: "Match the model to the work",
    description:
      "Routine tasks belong on a fast, lower-cost model; spend more only when the job needs stronger reasoning or code work.",
  },
]

const INSTRUCTION_PARTS = [
  ["Role", "State the work this agent owns and the result people expect."],
  [
    "Process",
    "Describe its usual method, including when it should ask for help.",
  ],
  ["Output", "Specify the contents and format of a finished result."],
  ["Boundaries", "List actions that need confirmation or must never happen."],
]

export function ConfigureAgent() {
  return (
    <div className="mt-10 text-base leading-7">
      <p className="max-w-2xl text-muted">
        A custom agent works best when it owns a repeatable job. Its team sets
        the access boundary, while the model, instructions, tools, and sandbox
        determine how it does the work and what each session costs.
      </p>

      <DocsMediaPlaceholder
        type="video"
        title="Create a focused custom agent"
        description="Record the full agent form, a first session, and the settings changed after reviewing that result."
        className="mt-12"
      />

      <section aria-labelledby="three-decisions" className="mt-16">
        <h2
          id="three-decisions"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          Make three decisions first
        </h2>
        <p className="mt-3 max-w-2xl text-muted">
          Don&apos;t begin with every tool and a page of instructions. Decide
          the job, owner, and model before filling out the form.
        </p>

        <ol className="mt-8 divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
          {FIRST_DECISIONS.map((item) => (
            <li key={item.title} className="p-5">
              <span className="text-xs font-semibold tracking-[0.12em] text-muted">
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

      <div className="mt-16 space-y-14 border-t border-border pt-14">
        <DocSection title="Create the agent">
          <p>
            Open <strong className="text-foreground">Agents</strong>, select{" "}
            <strong className="text-foreground">Create agent</strong>, then add
            a short name and description. Choose the name people will search
            for, and write a description that distinguishes this job from the
            other agents in the workspace.
          </p>
          <DocLink href="/w/agents/new">Create an agent</DocLink>
        </DocSection>

        <DocsMediaPlaceholder
          type="image"
          title="The custom agent configuration form"
          description="Show Basics, Model, Instructions, Tools, Team, Sub-agents, and Advanced settings at a readable scale."
        />

        <DocSection title="Choose a model for the job">
          <p>
            New sessions use the model selected here by default. Start with{" "}
            <strong className="text-foreground">DeepSeek V4 Flash</strong> for
            routine work where speed and cost matter; move to a stronger model
            for harder research or code work.
          </p>
          <p className="mt-3">
            Someone starting a session can still pick another model, so this
            setting should fit the agent&apos;s usual workload rather than every
            possible request.
          </p>
        </DocSection>

        <DocSection title="Write instructions the agent can follow">
          <p>
            Write the job and expected result first. Add another rule only after
            a real session exposes a repeatable mistake; long speculative
            prompts become hard to debug.
          </p>
          <dl className="mt-6 divide-y divide-border rounded-xl border border-border bg-surface">
            {INSTRUCTION_PARTS.map(([term, description]) => (
              <div
                key={term}
                className="grid gap-1 px-4 py-3 sm:grid-cols-[7rem_1fr] sm:gap-4"
              >
                <dt className="text-sm font-semibold text-foreground">
                  {term}
                </dt>
                <dd className="text-sm leading-6 text-muted">{description}</dd>
              </div>
            ))}
          </dl>
        </DocSection>

        <DocSection title="Give it only the tools it needs">
          <p>
            Tools let the agent read files, change code, run shell commands,
            search the web, and ask for input. Switch off any tool that has no
            place in this job.
          </p>
          <p className="mt-3">
            Team plugins come with the agent, but switching off an optional
            plugin here doesn&apos;t remove it from the rest of the team.
          </p>
          <DocLink href="/docs/agents/tools-and-sub-agents">
            Understand tools, sub-agents, and sandboxes
          </DocLink>
        </DocSection>

        <DocSection title="Keep access with the owning team">
          <p>
            Every agent belongs to one team and works only in that team&apos;s
            channels. Give ownership to the group that will review the results,
            since the agent also receives resources approved for that team.
          </p>
          <DocLink href="/docs/workspace-and-access/teams">
            Learn how teams control access
          </DocLink>
        </DocSection>

        <DocSection title="Set the runtime for the workload">
          <p>
            Use the default sandbox image for general work; choose the developer
            image when the job needs preinstalled developer tools. Start with a
            small sandbox, then raise its CPU, memory, or disk only after a
            session hits a resource limit.
          </p>
          <p className="mt-3">
            Saving the form doesn&apos;t create a sandbox. Hivy waits until the
            agent starts a session.
          </p>
        </DocSection>

        <section
          aria-labelledby="test-before-automation"
          className="rounded-xl border border-border bg-surface-secondary p-6"
        >
          <h2
            id="test-before-automation"
            className="text-lg font-semibold tracking-tight text-foreground"
          >
            Test one real task before automating it
          </h2>
          <p className="mt-2 max-w-xl text-sm leading-6 text-muted">
            Run one representative task in a team channel and inspect both the
            result and its cost. Fix the instructions or tool access before you
            put the agent into an automation.
          </p>
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
  const id = "configure-" + title.toLowerCase().replaceAll(" ", "-")

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
      className="mt-5 inline-flex rounded-sm text-sm font-medium text-foreground underline decoration-border underline-offset-4 transition-colors hover:text-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus"
    >
      {children}
    </Link>
  )
}
