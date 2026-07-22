import Link from "next/link"
import type { ReactNode } from "react"
import { AppIcon } from "@/components/icon"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

const AUTH_METHODS = [
  [
    "No authentication",
    "For a public endpoint or one protected by its network.",
  ],
  [
    "OAuth",
    "Each member signs in, or admins configure one organization identity.",
  ],
  ["Bearer token", "Hivy sends a private token in the Authorization header."],
  ["Custom header", "Hivy sends an API key under the header name you provide."],
  [
    "OAuth client credentials",
    "A shared machine identity for organization servers and unattended work.",
  ],
] as const

export function McpServers() {
  return (
    <div className="mt-10 text-base leading-7">
      <p className="max-w-2xl text-muted">
        Add a remote Model Context Protocol server when Hivy&apos;s connection
        catalog doesn&apos;t cover the tool you need. Hivy supports Streamable
        HTTP and HTTP/SSE endpoints; it does not run a local stdio server.
      </p>

      <section aria-labelledby="mcp-scope" className="mt-14">
        <h2
          id="mcp-scope"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          Start with the ownership boundary
        </h2>
        <div className="mt-6 grid gap-4 sm:grid-cols-2">
          <ScopeCard
            title="Personal server"
            icon="circle-user"
            description="You own the endpoint and its credential. Attach it only to agents you use."
          />
          <ScopeCard
            title="Organization server"
            icon="users"
            description="Admins own the endpoint, choose its identity policy, and grant it to teams or named agents."
          />
        </div>
      </section>

      <DocsMediaPlaceholder
        type="video"
        title="Add and test a custom MCP server"
        description="Add an HTTPS Streamable HTTP endpoint, choose a safe demo authentication method, test its health, and attach it to one agent. Do not record real credentials."
        className="mt-12"
      />

      <div className="mt-16 space-y-14 border-t border-border pt-14">
        <DocSection title="Add the endpoint">
          <ol className="space-y-4">
            <Step number="1" title="Name the server">
              Use the product or job people will recognize in an agent&apos;s
              tool list.
            </Step>
            <Step number="2" title="Enter its remote URL">
              Public endpoints must use HTTPS. Pick Streamable HTTP unless the
              server uses HTTP/SSE transport.
            </Step>
            <Step number="3" title="Choose authentication">
              Save the credential through the form; Hivy never needs it in a
              skill, prompt, or team environment variable.
            </Step>
            <Step number="4" title="Test before granting access">
              A health check confirms that Hivy can reach the server and finish
              its protocol handshake.
            </Step>
          </ol>
        </DocSection>

        <DocsMediaPlaceholder
          type="image"
          title="MCP endpoint, authentication, and access"
          description="Capture the MCP server row expanded with transport, authentication status, health, and team or agent assignments visible. Use a demo endpoint."
        />

        <DocSection title="Choose whose identity the server uses">
          <dl className="mt-5 divide-y divide-border rounded-xl border border-border bg-surface">
            {AUTH_METHODS.map(([name, description]) => (
              <div
                key={name}
                className="grid gap-1 px-4 py-3 sm:grid-cols-[11rem_1fr] sm:gap-5"
              >
                <dt className="text-sm font-semibold text-foreground">
                  {name}
                </dt>
                <dd className="text-sm leading-6 text-muted">{description}</dd>
              </div>
            ))}
          </dl>
          <p className="mt-4">
            Per-member identity keeps outside permissions tied to the person who
            starts the work. An organization identity gives authorized agents
            one credential that admins maintain, which also fits scheduled runs
            without a person present.
          </p>
          <p className="mt-3">
            Personal servers are available in chats you start and schedules you
            own. Use an organization server for Slack, webhook, or other runs
            that start without your personal session.
          </p>
        </DocSection>

        <DocSection title="Grant the smallest useful scope">
          <p>
            A personal server attaches to selected agents. For an organization
            server, admins can grant the server to a team or attach it directly
            to an agent, then remove either assignment without deleting the
            endpoint.
          </p>
          <p className="mt-3">
            The server&apos;s own authorization still applies after Hivy grants
            access. A tool cannot reach resources that its saved external
            identity cannot reach.
          </p>
          <DocLink href="/docs/workspace-and-access/teams">
            Review team access
          </DocLink>
        </DocSection>

        <section
          aria-labelledby="mcp-health"
          className="rounded-xl border border-border bg-surface-secondary p-6"
        >
          <AppIcon icon="activity" className="h-4 w-4 text-accent" />
          <h2
            id="mcp-health"
            className="mt-4 text-lg font-semibold tracking-tight text-foreground"
          >
            Re-test after endpoint or credential changes
          </h2>
          <p className="mt-2 max-w-xl text-sm leading-6 text-muted">
            A saved server can report healthy, degraded, unavailable, or not
            checked. Fix authentication first when sign-in expired; fix the URL
            or remote service when Hivy cannot connect at all.
          </p>
        </section>
      </div>
    </div>
  )
}

function ScopeCard({
  title,
  icon,
  description,
}: {
  title: string
  icon: "circle-user" | "users"
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

function Step({
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

function DocSection({
  title,
  children,
}: {
  title: string
  children: ReactNode
}) {
  const id = "mcp-" + title.toLowerCase().replaceAll(" ", "-")
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
