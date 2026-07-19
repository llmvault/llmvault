import Link from "next/link"
import { AppIcon } from "@/components/icon"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

export function WelcomeToHivy() {
  return (
    <div className="mt-10 text-base leading-7">
      <section aria-labelledby="agents-and-automations">
        <h2
          id="agents-and-automations"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          Agents and automations
        </h2>
        <p className="mt-3 max-w-2xl text-muted">
          Use an agent when a task needs judgment. Use an automation when that
          work should start after an event, at a set time, or from an HTTP
          webhook.
        </p>
      </section>

      <DocsMediaPlaceholder
        className="mt-8"
        type="video"
        title="See agents and automations work together"
        description="Record a 60-second overview: start a task with an agent, show its result, then select that agent for a scheduled automation."
      />

      <div className="mt-14 space-y-14 border-t border-border pt-14">
        <section aria-labelledby="start-with-an-agent">
          <p className="text-xs font-semibold tracking-[0.12em] text-muted uppercase">
            Agents
          </p>
          <h2
            id="start-with-an-agent"
            className="mt-2 text-xl font-semibold tracking-tight text-foreground"
          >
            Start with a real task
          </h2>
          <p className="mt-3 max-w-2xl text-muted">
            Pick an agent built for the result you want, then describe that
            result. A specialist already has instructions, tools, knowledge, and
            team context for its job, so it doesn’t spend time or model usage on
            unrelated work.
          </p>
          <DocsMediaPlaceholder
            className="mt-6"
            type="image"
            title="Start a session with the right agent"
            description="Capture the new-session composer at 100% zoom. Keep the team, agent, and model selectors visible, with a short task ready to send."
          />
          <Link
            href="/docs/run-your-first-agent"
            className="mt-5 inline-flex items-center gap-2 rounded-sm text-sm font-medium text-foreground transition-colors hover:text-accent focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-focus"
          >
            Run your first agent
            <AppIcon icon="arrow-right" className="h-4 w-4" />
          </Link>
        </section>

        <section aria-labelledby="automate-repeatable-work">
          <p className="text-xs font-semibold tracking-[0.12em] text-muted uppercase">
            Automations
          </p>
          <h2
            id="automate-repeatable-work"
            className="mt-2 text-xl font-semibold tracking-tight text-foreground"
          >
            Run repeatable work automatically
          </h2>
          <p className="mt-3 max-w-2xl text-muted">
            An automation doesn’t wait for someone to send a message. A
            connected-app event, schedule, or HTTP webhook starts the agent;
            Hivy keeps the run in the chosen team for review.
          </p>
          <DocsMediaPlaceholder
            className="mt-6"
            type="image"
            title="Choose how an automation starts"
            description="Capture the Automations page at 100% zoom. Show connected-app events, schedules, and HTTP webhooks in the same frame."
          />
          <Link
            href="/docs/automations/overview"
            className="mt-5 inline-flex items-center gap-2 rounded-sm text-sm font-medium text-foreground transition-colors hover:text-accent focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-focus"
          >
            Explore automations
            <AppIcon icon="arrow-right" className="h-4 w-4" />
          </Link>
        </section>

        <section
          aria-labelledby="team-scoped-agents"
          className="border-t border-border pt-10"
        >
          <h2
            id="team-scoped-agents"
            className="text-xl font-semibold tracking-tight text-foreground"
          >
            Give each team the agents it needs
          </h2>
          <p className="mt-3 max-w-2xl text-muted">
            Each agent belongs to one team, which can create as many specialists
            as its work requires. Workspace managers choose the tools,
            connections, knowledge, and team context available to them; agents
            don’t get access to the rest of the workspace.
          </p>
          <Link
            href="/docs/agents/configure-an-agent"
            className="mt-5 inline-flex items-center gap-2 rounded-sm text-sm font-medium text-foreground transition-colors hover:text-accent focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-focus"
          >
            Create and configure an agent
            <AppIcon icon="arrow-right" className="h-4 w-4" />
          </Link>
        </section>
      </div>
    </div>
  )
}
