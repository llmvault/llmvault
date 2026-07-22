import Link from "next/link"
import type { ReactNode } from "react"
import { AppIcon } from "@/components/icon"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

const MEMORY_FLOW = [
  {
    number: "01",
    title: "A session finishes",
    description:
      "Hivy checks settled work for facts, decisions, preferences, conventions, or findings worth carrying forward.",
  },
  {
    number: "02",
    title: "Related facts are combined",
    description:
      "When new evidence supports an existing memory, Hivy updates the durable fact instead of filling the list with copies.",
  },
  {
    number: "03",
    title: "The agent can recall it later",
    description:
      "Future sessions can search the agent's learned memories when earlier context fits the current task.",
  },
]

export function AgentMemories() {
  return (
    <div className="mt-10 text-base leading-7">
      <p className="max-w-2xl text-muted">
        Memories carry durable context between one agent&apos;s sessions without
        turning every transcript into permanent instructions. Review them on the
        installed agent, then edit or forget anything that should change.
      </p>

      <section aria-labelledby="how-memory-builds" className="mt-14">
        <h2
          id="how-memory-builds"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          How agent memory builds
        </h2>
        <p className="mt-3 max-w-2xl text-muted">
          Hivy looks for information that will matter after the session ends.
          Temporary progress and routine tool output do not need a memory, so
          many sessions leave nothing behind.
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

      <DocsMediaPlaceholder
        className="mt-12"
        type="video"
        title="Review what an agent learned"
        description="Open Agents, choose an installed agent, and select Memories. Edit one fictional memory, save it, then forget a second item and confirm the removal."
        bleed={false}
      />

      <div className="mt-16 space-y-14 border-t border-border pt-14">
        <DocSection title="Review one agent at a time">
          <p>
            Open <strong className="text-foreground">Agents</strong>, choose an
            installed agent, then select its{" "}
            <strong className="text-foreground">Memories</strong> tab. Each card
            shows the learned fact, its tags, and when Hivy saved it.
          </p>
          <p className="mt-3">
            An empty list is not an error. Memories appear after the agent has
            completed work with a durable fact worth keeping.
          </p>
          <DocLink href="/w/agents">Open Agents</DocLink>
        </DocSection>

        <DocsMediaPlaceholder
          type="image"
          title="Learned memories for one agent"
          description="Capture the Memories tab with three fictional facts, visible tags, relative dates, and the options menu open on one card."
        />

        <DocSection title="Edit wording without inventing a new fact">
          <p>
            Choose <strong className="text-foreground">Edit</strong> when the
            memory is useful but its wording is incomplete or wrong. Replace it
            with a short statement that the evidence supports, then save.
          </p>
          <p className="mt-3">
            Editing changes what the agent can recall. Put temporary directions
            in the current session instead of turning them into lasting memory.
          </p>
        </DocSection>

        <DocSection title="Forget context that should stop applying">
          <p>
            Choose <strong className="text-foreground">Forget</strong> when the
            fact is obsolete, sensitive, or belongs to another agent. Hivy asks
            for confirmation because the removal cannot be undone.
          </p>
          <p className="mt-3">
            Forgetting one agent&apos;s memory does not remove source material
            from a knowledge source, Sheet, Drive, or earlier session.
          </p>
        </DocSection>

        <section
          aria-labelledby="memory-boundary"
          className="rounded-xl border border-border bg-surface-secondary p-6"
        >
          <div className="flex items-start gap-3">
            <span className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-border bg-surface text-muted">
              <AppIcon icon="brain" className="h-4 w-4" />
            </span>
            <div>
              <h2
                id="memory-boundary"
                className="text-lg font-semibold tracking-tight text-foreground"
              >
                Memory belongs to the agent
              </h2>
              <p className="mt-2 max-w-xl text-sm leading-6 text-muted">
                Team access controls who can open the agent and its work, but
                memories do not automatically move to another agent on the same
                team. Each agent learns from its own sessions.
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
  const id = "memory-" + title.toLowerCase().replaceAll(" ", "-")
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
