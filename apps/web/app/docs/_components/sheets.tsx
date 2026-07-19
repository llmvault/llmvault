import Link from "next/link"
import type { ReactNode } from "react"
import { AppIcon } from "@/components/icon"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

const SHEET_BUILDING_BLOCKS = [
  {
    number: "01",
    title: "Sheet",
    description:
      "One database for work you expect to revisit, such as a lead list or inventory records.",
  },
  {
    number: "02",
    title: "Page",
    description:
      "A tab for one record type. A sales Sheet might have separate Companies and Contacts pages.",
  },
  {
    number: "03",
    title: "Field",
    description:
      "A typed column. Its type tells people and agents which values belong there.",
  },
  {
    number: "04",
    title: "Row",
    description:
      "One record that people and agents can edit or find again in a later session.",
  },
]

export function Sheets() {
  return (
    <div className="mt-10 text-base leading-7">
      <p className="max-w-2xl text-muted">
        Keep information that should outlive one session in a Sheet. People and
        agents can update the same records today, then find them again when the
        next task starts.
      </p>

      <DocsMediaPlaceholder
        type="video"
        title="Create a Sheet and update it with an agent"
        description="Record a new Sheet for a team. Ask an agent to add the fields and records, keep the live update visible, then open that Sheet from the main Sheets page."
        className="mt-12"
      />

      <section aria-labelledby="when-to-use-a-sheet" className="mt-16">
        <h2
          id="when-to-use-a-sheet"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          Use a Sheet when the information should last
        </h2>
        <p className="mt-3 max-w-2xl text-muted">
          Keep a table in the session when you only need a quick answer. Choose
          a Sheet when the team will return to the data, filter or edit it, and
          ask an agent to continue the work later. Because the Sheet belongs to
            a team rather than one session, it stays near the work it supports.
        </p>

        <ol className="mt-8 divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
          {SHEET_BUILDING_BLOCKS.map((item) => (
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
        type="image"
        title="A Sheet with pages, typed fields, and rows"
        description="Frame the complete Sheets panel with readable demo records. Include the Sheet selector and page tabs, field type icons, and every toolbar control from Search through Undo."
        className="mt-12"
      />

      <div className="mt-16 space-y-14 border-t border-border pt-14">
        <DocSection title="Create it yourself or ask an agent">
          <p>
            Open <strong className="text-foreground">Sheets</strong> from the
            workspace sidebar; Hivy groups each database under its team. If
            you know the columns you need, start with a blank Sheet. Otherwise,
            ask an agent with the Sheets connection to plan the structure and
            add the records.
          </p>
          <p className="mt-3">
            Tell the agent what one row represents and what you&apos;ll do with
            the result. For a lead database, you could ask for Company, Contact,
            Status, Owner, and Next step fields. That gives the agent enough
            context to pick field types instead of dropping everything into
            plain text columns.
          </p>
          <DocLink href="/w/sheets">Open Sheets</DocLink>
        </DocSection>

        <DocSection title="Use field types that match the work">
          <p>
            Field types change how a cell behaves. Use text or long text for
            words; numbers, checkboxes, select choices, and dates keep
            structured values tidy. Sheets also handle URLs, email addresses,
            phone numbers, attachments, and relations to rows on another page.
          </p>
          <p className="mt-3">
            Pages let connected record types live in the same Sheet without
            mixing their fields. Companies and Contacts, for example, work well
            as two pages with a relation between them. Start another Sheet when
            teammates would naturally look for a separate database.
          </p>
        </DocSection>

        <DocSection title="Find the records you need">
          <p>
            Search scans the current page, filters narrow it by field, and sort
            rules set the row order. Save a view if teammates need that setup
            again; it remembers hidden columns and column widths too.
          </p>
          <p className="mt-3">
            Agents query the same records directly. Ask, &quot;Which qualified
            leads have no next step?&quot; The agent can return only those rows
            or update them where they sit.
          </p>
        </DocSection>

        <DocSection title="Bring data in and take it out">
          <p>
            Import puts a CSV into an existing page after you check the columns
            Hivy detects and map each one to a Sheet field. A large file can
            keep running in the background while Hivy reports its progress.
          </p>
          <p className="mt-3">
            Select <strong className="text-foreground">Export</strong> to
            download the current page as a CSV. The file contains every field
            and row from the grid, ready for another system to read.
          </p>
        </DocSection>

        <DocSection title="Work alongside agents without losing control">
          <p>
            People see an agent&apos;s edits as they happen. Hivy keeps recent
            row writes, imports, and field changes in the operation log. Open{" "}
            <strong className="text-foreground">Undo</strong> to reverse the
            operation that caused the problem.
          </p>
          <p className="mt-3">
            Give the agent a narrow rule before a large update: name the rows it
            should change and the fields it must leave alone. It reads fresh
            data before a bulk write, so a teammate&apos;s recent edit
            doesn&apos;t get replaced by an old value.
          </p>
        </DocSection>

        <section
          aria-labelledby="sheet-access-follows-team"
          className="rounded-xl border border-border bg-surface-secondary p-6"
        >
          <div className="flex items-start gap-3">
            <span className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-border bg-surface text-muted">
              <AppIcon icon="shield-check" className="h-4 w-4" />
            </span>
            <div>
              <h2
                id="sheet-access-follows-team"
                className="text-lg font-semibold tracking-tight text-foreground"
              >
                Sheet access follows the team
              </h2>
              <p className="mt-2 max-w-xl text-sm leading-6 text-muted">
                Hivy keeps every Sheet inside the team where someone created
                it. A person needs access to that team. An agent also needs a
                session there and must have the Sheets connection before it can
                read or change the records.
              </p>
            </div>
          </div>
        </section>

        <DocSection title="Turn a Sheet into an app">
          <p>
            A grid won&apos;t suit every job. Ask Ricky to build an app over the
            Sheet when teammates need a dashboard, an approval queue, or a
            purpose-built editing screen; the records continue to live in the
            Sheet.
          </p>
          <DocLink href="/docs/sheets-and-apps/agent-built-apps">
            Build an app with Ricky
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
  const id = "sheets-" + title.toLowerCase().replaceAll(" ", "-")

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
