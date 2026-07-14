import Link from "next/link"
import type { ReactNode } from "react"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

const WORK_VIEWS = [
  ["Review", "Read code changes and attach feedback to a specific line."],
  ["Browser", "Load the session’s live preview and refresh it in place."],
  ["Files", "Browse repositories and open files from the session sandbox."],
  ["Canvas", "Open a visual artifact and comment on the preview itself."],
  ["Sheets", "Edit channel databases that remain available across sessions."],
  ["Apps", "Run an agent-built app inside Hivy or open it in another tab."],
  [
    "Subagents",
    "Read the activity stream for any task delegated by the parent agent.",
  ],
]

export function GeneratedWork() {
  return (
    <div className="mt-10 text-base leading-7">
      <p className="max-w-2xl text-muted">
        An agent&apos;s final message often points to work elsewhere in the
        session. Hivy keeps that work in a right panel, where you can inspect a
        file, preview, Sheet, app, Canvas artifact, code diff, or sub-agent run
        without leaving the task.
      </p>

      <DocsMediaPlaceholder
        type="video"
        title="Watch the work behind a finished artifact"
        description="Record the agent creating a file, opening a preview, updating a Sheet, delegating one task, and applying feedback."
        className="mt-12"
      />

      <section aria-labelledby="open-generated-work" className="mt-16">
        <h2
          id="open-generated-work"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          Open the work beside the session
        </h2>
        <p className="mt-3 max-w-2xl text-muted">
          Open the right panel beside the session and add the views needed for
          the current task. You can switch tabs or expand the panel without
          hiding the session history.
        </p>
        <dl className="mt-8 divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
          {WORK_VIEWS.map(([name, detail]) => (
            <div
              key={name}
              className="grid gap-1 px-4 py-3 sm:grid-cols-[7rem_1fr] sm:gap-4"
            >
              <dt className="text-sm font-semibold text-foreground">{name}</dt>
              <dd className="text-sm leading-6 text-muted">{detail}</dd>
            </div>
          ))}
        </dl>
      </section>

      <DocsMediaPlaceholder
        type="image"
        title="A session with multiple work views open"
        description="Place the session on the left and show Files, Browser, Sheets, Apps, and Subagents tabs in the right panel."
        className="mt-12"
      />

      <div className="mt-16 space-y-14 border-t border-border pt-14">
        <DocSection title="Inspect files and code changes">
          <p>
            Files connects to the active session&apos;s sandbox. Choose a
            repository, expand a folder, and open the file you need to inspect.
          </p>
          <p className="mt-3">
            When the agent changes code, Review shows the diff and accepts
            comments on individual lines. Send those comments as the next
            session message so the agent can address them against the current
            files.
          </p>
        </DocSection>

        <DocSection title="Open live previews in Browser">
          <p>
            When the agent starts a preview, its session shows a preview card.
            Select <strong className="text-foreground">Open</strong> to load the
            running app in Browser, where navigation and refresh controls remain
            available beside the session.
          </p>
          <p className="mt-3">
            The card points to a process inside that session&apos;s sandbox. If
            the process stops, ask the agent to restart it and then reopen the
            card.
          </p>
        </DocSection>

        <DocSection title="Review visual work in Canvas">
          <p>
            Canvas attaches visual artifacts such as webpages and presentations
            to the session. Open one at the viewport you need, select the area
            under review, and send that comment back to the agent.
          </p>
        </DocSection>

        <DocSection title="Keep long-term information in Sheets">
          <p>
            A Hivy Sheet is a channel database shared by agents and people.
            Because the Sheet belongs to the channel instead of one session,
            later tasks can read and update the same records.
          </p>
          <p className="mt-3">
            In Sheets, you can switch databases, manage pages and fields, or
            filter and sort rows while agent changes arrive live.
          </p>
          <DocLink href="/docs/sheets-and-apps/sheets">
            Learn how Sheets work
          </DocLink>
        </DocSection>

        <DocSection title="Turn data and services into an app">
          <p>
            An agent-built app can use a Hivy Sheet, your own database, or an
            external service behind any interface the team needs. Run it inside
            Hivy during the session, or open it in another tab.
          </p>
          <p className="mt-3">
            Choose{" "}
            <strong className="text-foreground">Ricky, App builder</strong> when
            the requested result is an app rather than a report or data table.
          </p>
          <DocLink href="/docs/sheets-and-apps/agent-built-apps">
            Build an app with an agent
          </DocLink>
        </DocSection>

        <DocSection title="Inspect delegated work separately">
          <p>
            Hivy gives each sub-agent task a card with its current status. Open
            that card for the helper&apos;s full activity stream, and use
            Subagents to switch between delegated runs.
          </p>
          <p className="mt-3">
            Those runs don&apos;t replace the parent agent; it still combines
            their results and finishes the original session.
          </p>
          <DocLink href="/docs/agents/tools-and-sub-agents">
            Configure sub-agents
          </DocLink>
        </DocSection>

        <section
          aria-labelledby="ask-for-change"
          className="rounded-xl border border-border bg-surface-secondary p-6"
        >
          <h2
            id="ask-for-change"
            className="text-lg font-semibold tracking-tight text-foreground"
          >
            Ask for the change where the work lives
          </h2>
          <p className="mt-2 max-w-xl text-sm leading-6 text-muted">
            Put feedback in the same session as the file, preview, Sheet, app,
            or Canvas artifact under review. The agent can change the existing
            result instead of rebuilding it from an empty workspace.
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
  const id = "generated-" + title.toLowerCase().replaceAll(" ", "-")
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
