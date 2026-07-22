import Link from "next/link"
import type { ReactNode } from "react"
import { AppIcon } from "@/components/icon"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

export function WorkspaceSettings() {
  return (
    <div className="mt-10 text-base leading-7">
      <p className="max-w-2xl text-muted">
        Workspace context gives every agent the same company facts. Team
        environment variables hold secret runtime values, while environment
        settings decide which local app ports receive preview URLs.
      </p>

      <section aria-labelledby="shared-context" className="mt-14">
        <h2
          id="shared-context"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          Give every agent a common starting point
        </h2>
        <p className="mt-3 max-w-2xl text-muted">
          Admins set the workspace name, logo, website, and company context in
          General settings. Hivy adds the name, website, and company context to
          each agent&apos;s runtime instructions.
        </p>
        <div className="mt-6 rounded-xl border border-border bg-surface p-5">
          <h3 className="font-semibold text-foreground">
            Company context should answer four questions
          </h3>
          <ul className="mt-4 space-y-2 text-sm leading-6 text-muted">
            <ContextItem>What does the company make or operate?</ContextItem>
            <ContextItem>Who does it serve?</ContextItem>
            <ContextItem>
              Which terms, products, and boundaries recur in the work?
            </ContextItem>
            <ContextItem>
              How should agents write when they represent the company?
            </ContextItem>
          </ul>
        </div>
        <p className="mt-4 max-w-2xl text-sm leading-6 text-muted">
          The website field adds a reference to the prompt; it does not crawl or
          index the site. Add a website knowledge source when agents need to
          search its pages.
        </p>
      </section>

      <DocsMediaPlaceholder
        type="video"
        title="Set workspace context and a team secret"
        description="Update the demo workspace context, add a described environment variable to one team, then start a session and show the agent recognizing the variable name without revealing its value."
        className="mt-12"
      />

      <div className="mt-16 space-y-14 border-t border-border pt-14">
        <DocSection title="Keep secret values with the team">
          <p>
            Open a team in <strong className="text-foreground">Settings</strong>
            , then add an environment variable with a name, value, and useful
            description. Hivy encrypts the value and never returns it to the
            settings page after saving.
          </p>
          <p className="mt-3">
            Sessions for that team receive the variable in their sandbox. The
            agent can see the name and description in its context, but its
            instructions forbid reading, printing, logging, or storing the
            value.
          </p>
        </DocSection>

        <DocsMediaPlaceholder
          type="image"
          title="Team environment variables with masked values"
          description="Capture one team with two demo variables. Show useful descriptions and masked saved values; never use a real secret."
        />

        <DocSection title="Expose only the preview ports you use">
          <p>
            Environments settings controls the local ports Hivy exposes through
            public sandbox preview URLs. The default list covers 3000, 5173,
            8000, and 8080; admins can save up to 20 distinct ports between 1
            and 65535.
          </p>
          <p className="mt-3">
            Port 7080 belongs to the Hivy runtime and cannot be selected. A port
            change applies only to new sandboxes, so recreate an existing
            sandbox before expecting a new preview route.
          </p>
        </DocSection>

        <DocSection title="Put each kind of context in the right place">
          <ul className="space-y-3 text-sm leading-6">
            <KindItem icon="users">
              Workspace context holds short company facts every agent needs.
            </KindItem>
            <KindItem icon="folder-open">
              Knowledge sources hold indexed material that selected teams can
              search.
            </KindItem>
            <KindItem icon="braces">
              Environment variables hold encrypted values used by programs and
              tools inside one team&apos;s sessions.
            </KindItem>
            <KindItem icon="sparkles">
              Skills hold reusable methods, never credentials.
            </KindItem>
          </ul>
          <DocLink href="/docs/knowledge-and-memory/knowledge-sources">
            Add searchable knowledge
          </DocLink>
        </DocSection>
      </div>
    </div>
  )
}

function ContextItem({ children }: { children: ReactNode }) {
  return (
    <li className="flex gap-3">
      <AppIcon icon="check" className="mt-1 h-4 w-4 shrink-0 text-accent" />
      <span>{children}</span>
    </li>
  )
}

function KindItem({
  icon,
  children,
}: {
  icon: "users" | "folder-open" | "braces" | "sparkles"
  children: ReactNode
}) {
  return (
    <li className="flex gap-3">
      <AppIcon icon={icon} className="mt-1 h-4 w-4 shrink-0 text-accent" />
      <span>{children}</span>
    </li>
  )
}

function DocSection({
  title,
  children,
}: {
  title: string
  children: ReactNode
}) {
  const id = "workspace-" + title.toLowerCase().replaceAll(" ", "-")
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
