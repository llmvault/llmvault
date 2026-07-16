import Link from "next/link"
import type { ReactNode } from "react"
import { AppIcon } from "@/components/icon"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

const SLACK_BEHAVIOR = [
  {
    title: "One Slack thread, one Hivy session",
    description:
      "Mention Hivy in a new Slack message to start a session; replies in its thread keep that session going.",
    icon: "messages-square" as const,
  },
  {
    title: "The Hivy team sets access",
    description:
      "The linked channel's team decides which agents can take the request and what those agents can use.",
    icon: "users" as const,
  },
  {
    title: "The request goes to a team agent",
    description:
      "Hivy chooses among the team's active agents; if none fits better, the default agent takes it.",
    icon: "bot" as const,
  },
]

export function SlackChannels() {
  return (
    <div className="mt-10 text-base leading-7">
      <p className="max-w-2xl text-muted">
        Mention Hivy where your team already works in Slack. Behind the thread,
        Hivy creates a session that keeps the request, agent work, result, and
        cost together in the workspace.
      </p>

      <section aria-labelledby="how-slack-work-maps-to-hivy" className="mt-14">
        <h2
          id="how-slack-work-maps-to-hivy"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          How Slack work maps to Hivy
        </h2>
        <p className="mt-3 max-w-2xl text-muted">
          Your team asks and replies in Slack, while Hivy runs the agent and
          stores the session under the linked channel. The channel&apos;s team
          rules still apply.
        </p>

        <ul className="mt-8 divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
          {SLACK_BEHAVIOR.map((item) => (
            <li key={item.title} className="flex gap-4 p-5">
              <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-border bg-surface-secondary text-accent">
                <AppIcon icon={item.icon} className="h-4 w-4" />
              </span>
              <div className="min-w-0">
                <h3 className="font-semibold text-foreground">{item.title}</h3>
                <p className="mt-1 text-sm leading-6 text-muted">
                  {item.description}
                </p>
              </div>
            </li>
          ))}
        </ul>
      </section>

      <DocsMediaPlaceholder
        type="image"
        title="A linked Slack thread and its Hivy session"
        description="Make this side-by-side image at 4K and 100% browser zoom. Put a demo Slack thread with its first @Hivy request on the left and the matching Hivy session on the right; hide workspace IDs, personal names, and unrelated channels."
        className="mt-12"
      />

      <div className="mt-16 space-y-14 border-t border-border pt-14">
        <DocStep number="1" title="Install and connect Slack">
          <p>
            A workspace Owner or Admin opens <strong>Connections</strong>,
            chooses the <strong>Slack</strong> connection, and adds it to the
            workspace. If no Slack connection exists, choose{" "}
            <strong>Connect</strong> under <strong>Required connections</strong>{" "}
            to open the authorization flow.
          </p>
          <p className="mt-3">
            Members can check Slack&apos;s status. Only an Owner or Admin can
            install or remove the connection and change its connection.
          </p>
          <DocLink href="/w/connections/slack">
            Open the Slack connection
          </DocLink>
        </DocStep>

        <DocStep number="2" title="Enable Slack for the team">
          <p>
            Go to <strong>Settings</strong> &gt; <strong>Teams</strong>, open
            the team that owns this work, and enable the Slack connection there.
          </p>
          <p className="mt-3">
            That team&apos;s agents can now use Slack. Agents on other teams
            don&apos;t gain access.
          </p>
          <DocLink href="/w/settings/teams">Open team settings</DocLink>
        </DocStep>

        <DocStep number="3" title="Link the Slack channel">
          <p>
            Select <strong>New channel</strong>, then choose its Hivy team,
            memory category, and default agent. Under{" "}
            <strong>Connect an external channel</strong>, pick Slack and the
            channel you want to link.
          </p>
          <p className="mt-3">
            Hivy joins a public Slack channel during setup. A private channel
            won&apos;t appear in the picker until you invite the Hivy Slack app
            to it.
          </p>
          <DocLink href="/w/channels/new">Create a channel</DocLink>
        </DocStep>

        <DocStep number="4" title="Start a session from Slack">
          <p>
            Mention the Hivy Slack app in the linked channel and include the
            whole request. Hivy replies in a thread and creates its session
            inside the linked Hivy channel.
          </p>
          <ExamplePrompt>
            @Hivy review the open launch tasks and list the blockers
          </ExamplePrompt>
          <p className="mt-3">
            After Hivy responds, reply in that thread to keep working in the
            same session. Use a new top-level mention for a separate task with
            its own context and cost.
          </p>
        </DocStep>

        <DocsMediaPlaceholder
          type="video"
          title="Connect Slack and complete a session"
          description="Record a 90 second walkthrough at 4K and 100% browser zoom. Connect the Slack connection, enable it for one team, link a public channel, send an @Hivy request, then open the resulting Hivy session."
          bleed={false}
        />

        <DocSection title="Agent access still follows the team">
          <p>
            A Slack link doesn&apos;t widen an agent&apos;s access. Hivy routes
            each new request among active agents on the channel&apos;s team, and
            the chosen agent keeps the limits from its team and configuration.
          </p>
          <p className="mt-3">
            If no team specialist fits better, the default agent handles the
            request. Change that fallback in the channel settings.
          </p>
          <DocLink href="/docs/agents/configure-an-agent">
            Configure an agent
          </DocLink>
        </DocSection>

        <DocSection title="Supported Slack channels">
          <p>
            Hivy accepts requests from public Slack channels and private
            channels that already include the Hivy Slack app. It ignores Direct
            messages and group direct messages.
          </p>
        </DocSection>

        <Troubleshooting />
      </div>
    </div>
  )
}

function DocStep({
  number,
  title,
  children,
}: {
  number: string
  title: string
  children: ReactNode
}) {
  const id = "slack-step-" + number + "-" + slugify(title)

  return (
    <section aria-labelledby={id}>
      <p className="text-xs font-semibold tracking-[0.12em] text-muted uppercase">
        Step {number}
      </p>
      <h2
        id={id}
        className="mt-2 text-xl font-semibold tracking-tight text-foreground"
      >
        {title}
      </h2>
      <div className="mt-3 max-w-2xl text-muted">{children}</div>
    </section>
  )
}

function DocSection({
  title,
  children,
}: {
  title: string
  children: ReactNode
}) {
  const id = "slack-" + slugify(title)

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

function ExamplePrompt({ children }: { children: ReactNode }) {
  return (
    <div className="mt-5 overflow-hidden rounded-xl border border-border bg-surface-secondary">
      <p className="border-b border-border px-4 py-2 text-xs font-semibold tracking-[0.1em] text-muted uppercase">
        Example
      </p>
      <p className="px-4 py-4 font-mono text-sm leading-6 text-foreground">
        {children}
      </p>
    </div>
  )
}

function Troubleshooting() {
  return (
    <section aria-labelledby="slack-troubleshooting">
      <h2
        id="slack-troubleshooting"
        className="text-xl font-semibold tracking-tight text-foreground"
      >
        If the channel does not connect
      </h2>
      <ul className="mt-5 max-w-2xl space-y-4 text-muted">
        <TroubleshootingItem title="Slack is unavailable during channel setup">
          Ask a workspace Owner or Admin to install and connect Slack, then
          enable the connection for your team.
        </TroubleshootingItem>
        <TroubleshootingItem title="A private channel is missing">
          Invite the Hivy Slack app to the private channel. Return to Hivy and
          reload the picker after Slack adds it.
        </TroubleshootingItem>
        <TroubleshootingItem title="Hivy says the channel is not connected">
          Link the Slack channel from the Hivy channel form, then mention the
          app again.
        </TroubleshootingItem>
        <TroubleshootingItem title="A thread does not receive a reply">
          Start with an explicit Hivy mention in a linked channel, and wait for
          Hivy&apos;s reply before sending the next request in its thread.
        </TroubleshootingItem>
      </ul>
    </section>
  )
}

function TroubleshootingItem({
  title,
  children,
}: {
  title: string
  children: ReactNode
}) {
  return (
    <li className="flex gap-3">
      <AppIcon
        icon="circle-check"
        className="mt-1.5 h-4 w-4 shrink-0 text-accent"
      />
      <div>
        <h3 className="font-semibold text-foreground">{title}</h3>
        <p className="mt-1 text-sm leading-6">{children}</p>
      </div>
    </li>
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

function slugify(value: string) {
  return value.toLowerCase().replaceAll(" ", "-")
}
