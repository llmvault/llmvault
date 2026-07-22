import Link from "next/link"
import type { ReactNode } from "react"
import { AppIcon } from "@/components/icon"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

const DRIVE_FLOW = [
  {
    icon: "search" as const,
    title: "Search the catalog",
    description:
      "The agent searches filenames, folders, file types, sizes, and dates. Search returns metadata and an exact asset ID, not the file body.",
  },
  {
    icon: "download" as const,
    title: "Download one asset",
    description:
      "The agent downloads that asset ID into a new sandbox path before reading or changing the file.",
  },
  {
    icon: "upload" as const,
    title: "Save the result",
    description:
      "When a file should survive the session, the agent uploads it from the sandbox into a Drive folder.",
  },
]

export function AgentDrive() {
  return (
    <div className="mt-10 text-base leading-7">
      <p className="max-w-2xl text-muted">
        A sandbox is temporary; an agent&apos;s Drive is not. Use Drive for
        files that the same agent should find in a later session, including
        uploaded inputs, reports, exports, and finished media.
      </p>

      <section aria-labelledby="drive-boundary" className="mt-14">
        <h2
          id="drive-boundary"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          Know which files will last
        </h2>
        <div className="mt-6 grid gap-4 sm:grid-cols-2">
          <BoundaryCard
            title="Sandbox files"
            icon="terminal"
            description="Working files live inside the current session's sandbox. They can disappear when that sandbox is replaced or cleaned up."
          />
          <BoundaryCard
            title="Drive files"
            icon="folder-open"
            description="Drive files belong to the agent and remain searchable across that agent's sessions."
          />
        </div>
        <p className="mt-4 max-w-2xl text-sm leading-6 text-muted">
          Hivy doesn&apos;t copy every sandbox file into Drive. Tell the agent
          which deliverables to keep, or ask it to upload a named path before
          the session ends.
        </p>
      </section>

      <DocsMediaPlaceholder
        className="mt-12"
        type="video"
        title="Carry a file from one session to the next"
        description="Upload a demo image in one session, have the agent create and save a related file, then open a second session with the same agent and retrieve both Drive assets."
      />

      <div className="mt-16 space-y-14 border-t border-border pt-14">
        <section aria-labelledby="drive-flow">
          <h2
            id="drive-flow"
            className="text-xl font-semibold tracking-tight text-foreground"
          >
            Search, download, then save
          </h2>
          <div className="mt-7 divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
            {DRIVE_FLOW.map((item) => (
              <div key={item.title} className="p-5">
                <AppIcon icon={item.icon} className="h-4 w-4 text-accent" />
                <h3 className="mt-4 font-semibold text-foreground">
                  {item.title}
                </h3>
                <p className="mt-2 text-sm leading-6 text-muted">
                  {item.description}
                </p>
              </div>
            ))}
          </div>
        </section>

        <DocsMediaPlaceholder
          type="image"
          title="A session using Agent Drive"
          description="Show drive_search returning a short result list, drive_download using one asset ID, and drive_upload saving the finished file. Keep the Files panel visible beside the session."
        />

        <DocSection title="Attach images to a request">
          <p>
            Use the plus button in the composer to attach an image. Hivy stores
            it in the selected agent&apos;s Drive, describes it for the model,
            and keeps the attachment with the message.
          </p>
          <p className="mt-3">
            An attachment belongs to the agent you selected before sending. If
            another agent needs it, ask the first agent to produce a shareable
            result or attach the file again for the other agent.
          </p>
          <DocLink href="/docs/agents/sessions">
            Read about session inputs
          </DocLink>
        </DocSection>

        <DocSection title="Keep Drive scope in mind">
          <p>
            Drive is agent-scoped. Team membership controls who can open that
            agent&apos;s sessions, but granting a team connection or knowledge
            source doesn&apos;t merge files between its agents.
          </p>
          <p className="mt-3">
            Use a Sheet for structured records that several people and agents
            should edit together. Drive fits files; Sheets fit shared rows.
          </p>
          <DocLink href="/docs/sheets-and-apps/sheets">
            Compare Drive with Sheets
          </DocLink>
        </DocSection>
      </div>
    </div>
  )
}

function BoundaryCard({
  title,
  icon,
  description,
}: {
  title: string
  icon: "terminal" | "folder-open"
  description: string
}) {
  return (
    <div className="rounded-xl border border-border bg-surface p-5">
      <AppIcon icon={icon} className="h-4 w-4 text-accent" />
      <h3 className="mt-4 font-semibold text-foreground">{title}</h3>
      <p className="mt-2 text-sm leading-6 text-muted">{description}</p>
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
  const id = "drive-" + title.toLowerCase().replaceAll(" ", "-")
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
      className="mt-4 inline-flex rounded-sm text-sm font-medium text-foreground underline decoration-border underline-offset-4 transition-colors hover:text-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus"
    >
      {children}
    </Link>
  )
}
