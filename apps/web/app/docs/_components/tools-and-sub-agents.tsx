import Link from "next/link"
import type { ReactNode } from "react"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

const TOOL_GROUPS = [
  [
    "Files and code",
    "Read files, write changes, apply patches, and inspect code.",
  ],
  ["Shell", "Run scripts or commands inside the session sandbox."],
  ["Image and web", "Create visuals, search the web, or read a specific page."],
  [
    "Orchestration",
    "Manage plans, request input, search sessions, or delegate.",
  ],
]

const SANDBOX_SIZES = [
  ["Nano", "1 CPU, 1 GB RAM, 5 GB disk"],
  ["Small", "1 CPU, 2 GB RAM, 10 GB disk"],
  ["Medium", "2 CPU, 4 GB RAM, 20 GB disk"],
  ["Large", "4 CPU, 8 GB RAM, 40 GB disk"],
]

export function ToolsAndSubAgents() {
  return (
    <div className="mt-10 text-base leading-7">
      <p className="max-w-2xl text-muted">
        Tool access, sub-agents, and sandbox resources solve different problems.
        Configure each one against a task the agent actually performs; extra
        access and compute make failures harder to explain and can raise cost.
      </p>

      <DocsMediaPlaceholder
        type="video"
        title="Build a parent agent with specialist sub-agents"
        description="Record the parent’s tool settings, two specialist sub-agents, and the point where their results return to the parent session."
        className="mt-12"
      />

      <div className="mt-16 space-y-14">
        <DocSection title="Tools control what the agent can do">
          <p>
            Runtime tools act inside the sandbox or Hivy. Connections and custom
            MCP servers add outside tools, while skills provide reusable
            instructions. Grant only the combination required by the job.
          </p>
          <dl className="mt-6 divide-y divide-border rounded-xl border border-border bg-surface">
            {TOOL_GROUPS.map(([term, description]) => (
              <div
                key={term}
                className="grid gap-1 px-4 py-3 sm:grid-cols-[9rem_1fr] sm:gap-4"
              >
                <dt className="text-sm font-semibold text-foreground">
                  {term}
                </dt>
                <dd className="text-sm leading-6 text-muted">{description}</dd>
              </div>
            ))}
          </dl>
          <p className="mt-4">
            Hivy selects every configurable built-in tool on a new custom agent.
            Remove the ones this role won&apos;t use; if you add a sub-agent,
            Hivy keeps the delegation tool switched on. Drive transfer tools
            stay on because they protect useful files from temporary sandbox
            storage.
          </p>
        </DocSection>

        <DocSection title="Team connections add external capabilities">
          <p>
            An agent receives the connections available to its team, which may
            expose a connected service or database. Switch off an optional
            connection on one agent when that role doesn&apos;t need it; the
            team grant remains available to its other agents.
          </p>
          <p className="mt-3">
            Catalog requirements work differently: Hivy blocks the team
            installation until every required connection is available, then
            keeps those required connections on for the installed agent.
          </p>
          <DocLink href="/docs/connections-and-skills/how-access-works">
            Learn how connections and team access work
          </DocLink>
        </DocSection>

        <DocsMediaPlaceholder
          type="image"
          title="Tools, sub-agents, and sandbox settings"
          description="Include the enabled tool chips, one expanded sub-agent, and the Advanced sandbox controls in the same frame."
        />

        <DocSection title="Use sub-agents for distinct specialties">
          <p>
            Give a sub-agent one bounded specialty and describe when the parent
            should call it. Its instructions and tool list should cover that
            assignment, not the parent&apos;s whole role.
          </p>
          <ol className="mt-5 space-y-4">
            <Guideline number="1" title="Separate by expertise">
              Create a specialist when a part of the job needs instructions or
              context that would distract the parent.
            </Guideline>
            <Guideline number="2" title="Choose its model independently">
              Let the sub-agent inherit the parent model unless its assignment
              has a different cost or capability requirement.
            </Guideline>
            <Guideline number="3" title="Grant its tools independently">
              Parent-only tools don&apos;t flow into the sub-agent; it receives
              the tool list saved on its own configuration.
            </Guideline>
            <Guideline number="4" title="Keep delegation worth the overhead">
              Don&apos;t split out work the parent already handles well. A
              separate run makes sense when the specialist needs a distinct
              context or can take a self-contained assignment.
            </Guideline>
          </ol>
          <p className="mt-5">
            Hivy shows each delegated task as a sub-agent card during the
            session. Open the card to inspect its progress while the parent
            coordinates the overall task.
          </p>
        </DocSection>

        <DocSection title="Sandboxes isolate execution by session">
          <p>
            Hivy provisions an isolated sandbox when a session starts, using the
            image and size saved on the agent. Files, commands, and runtime
            tools operate inside that sandbox.
          </p>
          <p className="mt-3">
            Use the <strong className="text-foreground">Default</strong> image
            for general work; choose{" "}
            <strong className="text-foreground">Developer</strong> when the
            agent needs preinstalled developer tooling.
          </p>
          <dl className="mt-6 divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
            {SANDBOX_SIZES.map(([name, specs]) => (
              <div key={name} className="p-4">
                <dt className="text-sm font-semibold text-foreground">
                  {name}
                </dt>
                <dd className="mt-1 text-sm text-muted">{specs}</dd>
              </div>
            ))}
          </dl>
          <p className="mt-4">
            Start with the smallest size your workspace tier allows. Move up
            only after a session runs short of memory, disk, or processing
            capacity; lifetime credit deposits permanently unlock larger sizes.
          </p>
        </DocSection>

        <section
          aria-labelledby="keep-agent-legible"
          className="rounded-xl border border-border bg-surface-secondary p-6"
        >
          <h2
            id="keep-agent-legible"
            className="text-lg font-semibold tracking-tight text-foreground"
          >
            Add capability only when the job earns it
          </h2>
          <p className="mt-2 max-w-xl text-sm leading-6 text-muted">
            Begin with one agent and the smallest available sandbox. Let a real
            session expose the missing tool or resource before you add it.
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
  const id = "tools-" + title.toLowerCase().replaceAll(" ", "-")
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

function Guideline({
  number,
  title,
  children,
}: {
  number: string
  title: string
  children: ReactNode
}) {
  return (
    <li className="grid gap-3 sm:grid-cols-[2rem_1fr]">
      <span className="text-xs font-semibold tracking-[0.12em] text-muted">
        {number.padStart(2, "0")}
      </span>
      <div>
        <h3 className="text-sm font-semibold text-foreground">{title}</h3>
        <p className="mt-1 text-sm leading-6 text-muted">{children}</p>
      </div>
    </li>
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
