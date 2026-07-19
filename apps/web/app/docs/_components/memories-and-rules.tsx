import Link from "next/link"
import type { ReactNode } from "react"
import { AppIcon } from "@/components/icon"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

const MEMORY_FLOW = [
  {
    number: "01",
    title: "A session leaves evidence",
    description:
      "Hivy checks completed work for decisions, preferences, conventions, commitments, or findings worth keeping.",
  },
  {
    number: "02",
    title: "Hivy consolidates related facts",
    description:
      "Hivy updates an existing memory when new evidence supports the same fact instead of saving another copy.",
  },
  {
    number: "03",
    title: "Later sessions receive it",
    description:
      "A new session gets the agent's active rules before relevant memories from earlier work.",
  },
]

const MEMORY_ACTIONS = [
  ["Confirm", "Mark the memory as verified and record fresh support for it."],
  [
    "Edit",
    "Replace incorrect wording with a person-verified version; Hivy keeps the earlier record in its history.",
  ],
  [
    "Pin as rule",
    "Copy the memory into an active rule that every session for the agent must follow.",
  ],
  [
    "Delete",
    "Remove the memory and block Hivy from learning the same content again during consolidation.",
  ],
]

export function MemoriesAndRules() {
  return (
    <div className="mt-10 text-base leading-7">
      <p className="max-w-2xl text-muted">
        Memories carry useful context between sessions without saving every
        message forever. Hivy learns memories from completed work, while people
        write rules that the agent must follow in every session.
      </p>

      <section
        aria-labelledby="memories-and-rules-difference"
        className="mt-14"
      >
        <h2
          id="memories-and-rules-difference"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          Use memory for context and rules for control
        </h2>
        <div className="mt-7 divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
          <div className="p-5">
            <span className="text-xs font-semibold tracking-[0.12em] text-muted uppercase">
              Hivy learns these
            </span>
            <h3 className="mt-4 font-semibold text-foreground">Memories</h3>
            <p className="mt-2 text-sm leading-6 text-muted">
              Facts, decisions, preferences, conventions, workarounds, or
              commitments that Hivy learns from sessions. New evidence can
              correct or strengthen them.
            </p>
          </div>
          <div className="p-5">
            <span className="text-xs font-semibold tracking-[0.12em] text-muted uppercase">
              A person writes these
            </span>
            <h3 className="mt-4 font-semibold text-foreground">Rules</h3>
            <p className="mt-2 text-sm leading-6 text-muted">
              Instructions for the agent, such as “Always request
              approval before issuing a refund.” Hivy sends every active rule
              into each session before learned context.
            </p>
          </div>
        </div>
      </section>

      <DocsMediaPlaceholder
        className="mt-12"
        type="video"
        title="Review and correct what an agent remembers"
        description="Capture an admin opening Settings > Memories and selecting an agent. Show Confirm on one memory, Edit on another, Pin as rule, and Delete on an outdated item. The memory kind, verification state, active rules, and each action label must remain readable."
        bleed={false}
      />

      <section aria-labelledby="how-memory-builds" className="mt-16">
        <h2
          id="how-memory-builds"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          How agent memory builds
        </h2>
        <p className="mt-3 max-w-2xl text-muted">
          Hivy ignores routine output, temporary progress, machine details,
          secrets, and work that has not settled yet. Many sessions produce no
          memory, and that&apos;s normal.
        </p>
        <ol className="mt-8 divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
          {MEMORY_FLOW.map((item) => (
            <li key={item.title} className="p-5">
              <span className="text-xs font-semibold tracking-[0.12em] text-muted uppercase">
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
        <DocSection title="Tell Hivy what an agent should remember">
          <p>
            An agent&apos;s <strong>Category</strong> tells Hivy which
            information belongs in memory. When you create an agent, choose{" "}
            Customer Support, Account / Client, Engineering, Operations, Sales,
            Marketing, People / HR, or General.
          </p>
          <p className="mt-3">
            Every specialized category starts with a memory mission. Open the
            agent under <strong>Settings</strong> to read or change its{" "}
            <strong>Memory mission</strong>. The Engineering mission, for
            example, asks Hivy to keep decisions, conventions, incident causes,
            and reusable fixes while ignoring temporary sandbox output.
          </p>
          <DocLink href="/w/agents">Open Agents</DocLink>
        </DocSection>

        <DocsMediaPlaceholder
          type="image"
          title="Agent memory mission"
          description="Capture one agent's settings with Category and Memory mission in the same frame. Use Engineering or Account / Client, and make the mission's keep and ignore instructions readable."
        />

        <DocSection title="Review one agent at a time">
          <p>
            Open <strong>Settings</strong>, select <strong>Memories</strong>,
            then choose an agent. Active rules appear above learned memories.
            Each memory shows its kind, evidence count, related entities,
            verification state, and an expiry date when it has one.
          </p>
          <p className="mt-3">
            Members may read memory for agents they can access. Only workspace
            owners and admins may change memories or rules.
          </p>
          <DocLink href="/w/settings/memories">Open Memories</DocLink>
        </DocSection>

        <DocSection title="Fix a bad memory at the source">
          <dl className="divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
            {MEMORY_ACTIONS.map(([action, result]) => (
              <div
                key={action}
                className="grid gap-1 px-4 py-4 sm:grid-cols-[8rem_1fr] sm:gap-5"
              >
                <dt className="text-sm font-semibold text-foreground">
                  {action}
                </dt>
                <dd className="text-sm leading-6 text-muted">{result}</dd>
              </div>
            ))}
          </dl>
        </DocSection>

        <DocSection title="Reserve rules for standing instructions">
          <p>
            Add a rule only when every session for the agent should follow it.
            Write an instruction that someone can verify, and leave temporary
            task details out. You cannot edit a rule in place; delete it and add
            a replacement when the wording changes.
          </p>
          <p className="mt-3">
            Choose <strong>Deactivate rule</strong> to stop applying an
            instruction while keeping its record. Use{" "}
            <strong>Pin as rule</strong>
            when a learned memory needs to become mandatory.
          </p>
        </DocSection>

        <DocsMediaPlaceholder
          type="image"
          title="Rules and memories for one agent"
          description="Capture Settings > Memories with one agent selected. Include two active rules plus memories of different kinds; one memory must show Verified and another must show more than one confirmation. Use fictional company information and keep the action menus out of the frame."
        />

        <section
          aria-labelledby="memory-safety-guidance"
          className="rounded-xl border border-border bg-surface-secondary p-6"
        >
          <div className="flex items-start gap-3">
            <span className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-border bg-surface text-muted">
              <AppIcon icon="shield-check" className="h-4 w-4" />
            </span>
            <div>
              <h2
                id="memory-safety-guidance"
                className="text-lg font-semibold tracking-tight text-foreground"
              >
                Pick the category before sensitive work starts
              </h2>
              <p className="mt-2 max-w-xl text-sm leading-6 text-muted">
                Each category has its own privacy boundary. Customer Support
                avoids details about individual customers; Account / Client may
                keep client-specific context. People / HR excludes personal,
                medical, compensation, and performance information. Read the
                Memory mission before anyone uses the agent for sensitive
                work.
              </p>
            </div>
          </div>
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
      className="mt-5 inline-flex items-center gap-2 rounded-sm text-sm font-medium text-foreground transition-colors hover:text-accent focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-focus"
    >
      {children}
      <AppIcon icon="arrow-right" className="h-4 w-4" />
    </Link>
  )
}
