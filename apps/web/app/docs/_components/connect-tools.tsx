import Link from "next/link"
import type { ReactNode } from "react"
import { AppIcon } from "@/components/icon"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

const SETUP_STEPS = [
  {
    number: "1",
    title: "Choose a provider",
    description:
      "The Connections page lists every supported external service and database provider.",
  },
  {
    number: "2",
    title: "Create an instance",
    description:
      "Sign in to an external account, or enter the details for a supported database.",
  },
  {
    number: "3",
    title: "Name duplicates",
    description:
      "Add multiple instances of the same provider and give each one a clear name.",
  },
  {
    number: "4",
    title: "Select resources",
    description:
      "Limit each instance to the repositories or provider resources its teams need.",
  },
  {
    number: "5",
    title: "Enable it for a team",
    description:
      "Open team settings and switch on the exact connection instance the team may use, then turn it off for any agent that does not need it.",
  },
]

export function ConnectTools() {
  return (
    <div className="mt-10 text-base leading-7">
      <p className="max-w-2xl text-muted">
        Pick a provider, create one or more concrete connection instances, and
        grant only the right instances to each team. Generated MCP tools become
        available automatically with the grant.
      </p>

      <DocsMediaPlaceholder
        className="mt-12"
        type="video"
        title="Set up and grant a connection"
        description="Record the full admin flow at a readable zoom. Open Connections, add a second instance of one provider, name it, select allowed resources, and grant that exact instance to one team."
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

        <DocLink href="/w/connections">Open Connections</DocLink>
      </section>

      <div className="mt-16 space-y-14 border-t border-border pt-14">
        <DocSection title="Start with the provider">
          <p>
            Search <strong>Connections</strong> by provider, then connect the
            account that should perform the work. Existing instances do not
            prevent you from adding another.
          </p>
          <p className="mt-3">
            Name additional instances so team settings clearly distinguish
            production, staging, client, or regional accounts.
          </p>
        </DocSection>

        <DocsMediaPlaceholder
          type="image"
          title="Choose a provider and review existing instances"
          description="Show the Connections page with one provider that already has two named instances and its Add another action visible."
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
            until its team has a grant for that exact connection instance.
          </p>
        </DocSection>

        <DocSection title="Connect databases with a read boundary">
          <p>
            Database connections work with PostgreSQL, MySQL, MongoDB, and
            Redis. Hivy checks the connection URL, reads the database structure,
            then displays the objects you can permit.
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

        <DocSection title="Generated MCP tools follow the grant">
          <p>
            Granting a connection makes its generated MCP tools available to
            that team&apos;s agents automatically.
          </p>
          <p className="mt-3">
            Team-owned skills and workspace skills granted to the team remain
            separate and editable through Settings → Skills.
          </p>
        </DocSection>

        <DocSection title="Enable the connection for the owning team">
          <p>
            Open <strong>Settings</strong>, choose <strong>Teams</strong>, and
            select the team that owns the work. Switching the connection on
            there gives it to every agent on that team.
          </p>
          <p className="mt-3">
            Installation alone never gives an agent access. Each team needs its
            own grant, even when several teams use the same workspace
            connection.
          </p>
          <p className="mt-3">
            To narrow access further, open an agent&apos;s settings and switch
            off an optional connection for that agent. Required catalog
            connections remain on until you uninstall the catalog agent.
          </p>
          <DocLink href="/w/settings/teams">Open team settings</DocLink>
        </DocSection>

        <DocsMediaPlaceholder
          type="image"
          title="Enable an installed connection for one team"
          description="Use a team settings page with the Connections section visible. Show the switch in its on state for one installed connection, with the selected team's name still in view."
        />

        <DocSection title="Reconnect, remove, or disconnect">
          <p>
            Select <strong>Reconnect</strong> when a provider token expires or
            the account settings change. Team grants stay in place while the
            account signs in again.
          </p>
          <p className="mt-3">
            Need to stop one team? Switch off its grant. If nobody in the
            workspace needs the instance, revoke every team grant and then
            disconnect it. Hivy blocks revocation when an installed catalog
            agent requires that provider.
          </p>
          <p className="mt-3">
            Select <strong>Disconnect</strong> only after removing its team
            grants. Hivy then revokes that saved workspace connection.
          </p>
          <DocLink href="/docs/connections-and-skills/how-access-works">
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
                fix the connection scope or connection access before adding an
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
