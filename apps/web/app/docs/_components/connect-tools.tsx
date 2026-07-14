import Link from "next/link"
import type { ReactNode } from "react"
import { AppIcon } from "@/components/icon"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

const SETUP_STEPS = [
  {
    number: "1",
    title: "Choose the plugin for the job",
    description:
      "Its detail page lists the work it handles, the skills it adds, and every connection or resource it needs.",
  },
  {
    number: "2",
    title: "Connect what it requires",
    description:
      "Use Connect for each missing external account, or enter the connection details for a supported database.",
  },
  {
    number: "3",
    title: "Add the plugin",
    description:
      "Add stays unavailable until Hivy can reach everything the plugin requires.",
  },
  {
    number: "4",
    title: "Select resources",
    description:
      "Limit the connection to the repositories or other provider resources that agents need.",
  },
  {
    number: "5",
    title: "Enable it for a team",
    description:
      "Open the owning team's settings and switch on the installed plugin.",
  },
]

export function ConnectTools() {
  return (
    <div className="mt-10 text-base leading-7">
      <p className="max-w-2xl text-muted">
        Pick the plugin for the work, then connect only the account it needs.
        Once an admin grants that plugin to a team, its agents can use the
        account through the plugin&apos;s skills.
      </p>

      <DocsMediaPlaceholder
        className="mt-12"
        type="video"
        title="Set up one plugin and its connection"
        description="Record the full admin flow at a readable zoom. Open Plugins, choose one with a missing connection, sign in to the service, use Add, select the allowed resources, and switch the plugin on for one team."
      />

      <section aria-labelledby="connect-a-tool" className="mt-14">
        <h2
          id="connect-a-tool"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          Connect a tool in five steps
        </h2>
        <p className="mt-3 max-w-2xl text-muted">
          Only a workspace owner or admin can finish these steps.
        </p>

        <ol className="mt-8 divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
          {SETUP_STEPS.map((step) => (
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

        <DocLink href="/w/plugins">Open Plugins</DocLink>
      </section>

      <div className="mt-16 space-y-14 border-t border-border pt-14">
        <DocSection title="Start with the plugin, not the account">
          <p>
            Search <strong>Plugins</strong> by name or category, then open the
            result that matches the work. Its detail page names the skills it
            adds and the external services those skills call.
          </p>
          <p className="mt-3">
            Starting there prevents stray connections: if no chosen plugin needs
            an account, don&apos;t connect it.
          </p>
        </DocSection>

        <DocsMediaPlaceholder
          type="image"
          title="Choose a plugin and review its requirements"
          description="Place the Plugins catalog beside one plugin detail page. Keep the plugin name, description, Required connections, examples, and skills large enough to read."
        />

        <DocSection title="Authorize the external service">
          <p>
            Select <strong>Connect</strong> beside a missing requirement. Most
            services open an OAuth sign-in; API key and basic-auth services ask
            for their credentials inside Hivy instead.
          </p>
          <p className="mt-3">
            Use the company account that agents should work through. A
            successful sign-in creates the connection, but no agent can call it
            until its team has the matching plugin.
          </p>
        </DocSection>

        <DocSection title="Connect databases with a read boundary">
          <p>
            Database plugins work with PostgreSQL, MySQL, MongoDB, and Redis.
            Hivy checks the connection URL, reads the database structure, then
            displays the objects you can permit.
          </p>
          <p className="mt-3">
            Use a read-only database user or replica. For SQL, permit only the
            needed schemas and tables, then mask fields agents shouldn&apos;t
            receive. MongoDB offers collection and field controls; Redis uses
            allowed key patterns.
          </p>
        </DocSection>

        <DocsMediaPlaceholder
          type="image"
          title="Set a database access policy"
          description="Use the database policy screen after inspection. Show the permitted schemas, tables or collections, and masked fields; the demo database must contain no real credentials or customer data."
        />

        <DocSection title="Add the plugin after requirements are ready">
          <p>
            <strong>Add</strong> stays disabled while a required connection is
            missing. When an admin selects it, Hivy installs the plugin&apos;s
            skills in the workspace catalog.
          </p>
          <p className="mt-3">
            Some plugins then ask for provider resources. A GitHub plugin can
            name the repositories agents may open, and Hivy saves that choice as
            the connection&apos;s default scope.
          </p>
        </DocSection>

        <DocSection title="Enable the plugin for the owning team">
          <p>
            Open <strong>Settings</strong>, choose <strong>Teams</strong>, and
            select the team that owns the work. Switching the plugin on there
            gives it to every agent on that team; you can still turn off an
            optional plugin for one agent.
          </p>
          <p className="mt-3">
            Installation alone never gives an agent access. Each team needs its
            own grant, even when several teams use the same workspace
            connection.
          </p>
          <DocLink href="/w/settings/teams">Open team settings</DocLink>
        </DocSection>

        <DocsMediaPlaceholder
          type="image"
          title="Enable an installed plugin for one team"
          description="Use a team settings page with the Plugins section visible. Show the switch in its on state for one installed plugin, with the selected team's name still in view."
        />

        <DocSection title="Reconnect, remove, or disconnect">
          <p>
            Select <strong>Reconnect</strong> when a provider token expires or
            the account settings change. The plugin installation and team grants
            stay in place while the account signs in again.
          </p>
          <p className="mt-3">
            Need to stop one team? Switch off its grant. If nobody in the
            workspace needs the plugin, select <strong>Remove</strong> and Hivy
            will clear every team grant. Hivy blocks removal when an active
            agent requires the plugin or when the plugin is always on or locked.
          </p>
          <p className="mt-3">
            Select <strong>Disconnect</strong> only after removing the plugin
            that uses the external account. Hivy then revokes the saved
            workspace connection.
          </p>
          <DocLink href="/docs/plugins-and-connections/how-access-works">
            Understand connections and team access
          </DocLink>
        </DocSection>

        <section
          aria-labelledby="check-the-scope"
          className="rounded-xl border border-border bg-surface-secondary p-6"
        >
          <div className="flex items-start gap-3">
            <span className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-border bg-surface text-muted">
              <AppIcon icon="check" className="h-4 w-4" />
            </span>
            <div>
              <h2
                id="check-the-scope"
                className="text-lg font-semibold tracking-tight text-foreground"
              >
                Test the access with one real task
              </h2>
              <p className="mt-2 max-w-xl text-sm leading-6 text-muted">
                Start one session with an agent on the enabled team and ask it
                to read from the connected service. Check the result against the
                permitted resources; if the agent sees too much or too little,
                fix the connection scope or plugin access before adding an
                automation.
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
