import Link from "next/link"
import type { ReactNode } from "react"
import { AppIcon } from "@/components/icon"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

const SOURCE_TYPES = [
  {
    title: "GitHub",
    description:
      "Make selected repositories searchable when an agent needs code or engineering context.",
  },
  {
    title: "Notion",
    description:
      "Index chosen pages and databases without giving agents the whole Notion workspace.",
  },
  {
    title: "Slack",
    description:
      "Keep decisions from selected resources available after a Slack thread has gone quiet.",
  },
  {
    title: "Linear",
    description: "Let agents search work from the Linear teams you select.",
  },
  {
    title: "Website",
    description:
      "Discover a site, then choose the pages or sections agents may search.",
  },
]

const ADD_SOURCE_STEPS = [
  {
    number: "1",
    title: "Pick the source",
    description:
      "Choose a connected service or enter a website. A connected service reads through an existing workspace connection.",
  },
  {
    number: "2",
    title: "Set a narrow scope",
    description:
      "Select only the repositories, pages, databases, Slack channels, Linear teams, or website paths that answer the questions you have in mind.",
  },
  {
    number: "3",
    title: "Choose the teams",
    description:
      "Select each team whose agents may search this source. Hivy requires at least one team.",
  },
  {
    number: "4",
    title: "Wait for indexing",
    description:
      "Hivy reads the chosen content in the background. Documents become searchable as the index fills.",
  },
]

export function KnowledgeSources() {
  return (
    <div className="mt-10 text-base leading-7">
      <p className="max-w-2xl text-muted">
        Agents can answer questions from the material your company already keeps
        in GitHub, Notion, Slack, Linear, or on a website. You decide what Hivy
        indexes and which teams may search it.
      </p>

      <section
        aria-labelledby="knowledge-is-not-a-connection"
        className="mt-14"
      >
        <h2
          id="knowledge-is-not-a-connection"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          Knowledge is context, not a live tool
        </h2>
        <p className="mt-3 max-w-2xl text-muted">
          A connection lets an agent read or change data in an external service.
          A knowledge source indexes a chosen set of content for search. You may
          connect the same service both ways, but search and live actions remain
          separate.
        </p>
        <DocLink href="/docs/connections-and-skills/connect-tools">
          Compare knowledge with connections
        </DocLink>
      </section>

      <DocsMediaPlaceholder
        className="mt-12"
        type="video"
        title="Add your first knowledge source"
        description="Capture an admin opening Settings > Knowledge and choosing Add source. Show the provider, a narrow resource selection, the Teams field, and the source status after saving. End on the source list as indexing starts; every label and selected resource must remain readable."
        bleed={false}
      />

      <section aria-labelledby="supported-sources" className="mt-16">
        <h2
          id="supported-sources"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          Bring in the sources agents need
        </h2>
        <div className="mt-7 divide-y divide-border border-y border-border">
          {SOURCE_TYPES.map((source) => (
            <div
              key={source.title}
              className="grid gap-1 py-5 sm:grid-cols-[8rem_1fr] sm:gap-8"
            >
              <h3 className="font-semibold text-foreground">{source.title}</h3>
              <p className="text-sm leading-6 text-muted">
                {source.description}
              </p>
            </div>
          ))}
        </div>
      </section>

      <section aria-labelledby="add-a-source" className="mt-16">
        <h2
          id="add-a-source"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          Add a source in four steps
        </h2>
        <p className="mt-3 max-w-2xl text-muted">
          Workspace owners and admins add sources under Settings. If the content
          lives in an external service, connect that service before you start.
        </p>

        <ol className="mt-8 divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
          {ADD_SOURCE_STEPS.map((step) => (
            <li key={step.title} className="flex gap-4 p-5">
              <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-default text-sm font-semibold text-foreground">
                {step.number}
              </span>
              <div>
                <h3 className="font-semibold text-foreground">{step.title}</h3>
                <p className="mt-1 text-sm leading-6 text-muted">
                  {step.description}
                </p>
              </div>
            </li>
          ))}
        </ol>
        <DocLink href="/w/knowledge/new">Add a source</DocLink>
      </section>

      <div className="mt-16 space-y-14 border-t border-border pt-14">
        <DocSection title="Keep each source narrow">
          <p>
            Give the source a name that says what it contains, such as
            Engineering repositories or Support handbook. Include only the
            resources that belong under that name; tighter scope makes search
            results easier to judge and access easier to review.
          </p>
          <p className="mt-3">
            For a website, enter the URL and run discovery before selecting
            individual pages or complete sections. With a connected service,
            Hivy lists the repositories, pages, Slack channels, or Linear teams
            that the connection can read.
          </p>
        </DocSection>

        <DocsMediaPlaceholder
          type="image"
          title="Select a focused source scope"
          description="Capture the Add knowledge source form after choosing a provider. The source name, resource picker, Teams field, and a short resource selection must fit in one readable frame. Use demo content."
        />

        <DocSection title="Check the index before relying on it">
          <p>
            The source list reports the source state and indexed document count.
            A large first sync may still run after its earliest documents become
            searchable, so check the count before treating the source as
            complete.
          </p>
          <p className="mt-3">
            Choose <strong>View documents</strong> from the source menu to check
            indexed titles and links. The list should match the scope you meant
            to add.
          </p>
          <DocLink href="/w/knowledge">
            Open Knowledge settings
          </DocLink>
        </DocSection>

        <DocSection title="Ask a question, not a search query">
          <p>
            Tell the agent to search company knowledge, then ask the question in
            plain language. “What did we decide about the launch date?” works;
            so does “Find the support escalation policy.”
          </p>
          <p className="mt-3">
            Hivy refreshes connected content on a schedule. Use the
            provider&apos;s connection when you need its current record or want
            the agent to change something there.
          </p>
          <DocLink href="/docs/agents/sessions">
            Read about agent sessions
          </DocLink>
        </DocSection>

        <section
          aria-labelledby="source-starting-point"
          className="rounded-xl border border-border bg-surface-secondary p-6"
        >
          <div className="flex items-start gap-3">
            <span className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-border bg-surface text-muted">
              <AppIcon icon="check" className="h-4 w-4" />
            </span>
            <div>
              <h2
                id="source-starting-point"
                className="text-lg font-semibold tracking-tight text-foreground"
              >
                Start with a question your team asks often
              </h2>
              <p className="mt-2 max-w-xl text-sm leading-6 text-muted">
                Add only enough material to answer that question, then grant the
                source to the team that owns the work. Test it in a session. If
                the answer lacks context, add the specific resource that holds
                the missing information.
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
