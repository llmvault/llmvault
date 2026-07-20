import type { ReactNode } from "react"
import { Button, Link } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import {
  LandingFooter,
  LandingHero,
} from "../../home/_components/landing-shared"
import {
  SlackReactionMockup,
  SlackThreadContinuityMockup,
  SlackWatchMockup,
  SlackWorkspaceMockup,
} from "./slack-previews"
import { SlackMemoryPreview, TeamTagUseCases } from "./slack-use-case-previews"

const routingExamples = [
  { channel: "#product-support", agent: "Support agent", icon: "headset" },
  { channel: "#incidents", agent: "Reliability agent", icon: "radar" },
  { channel: "#revenue-ops", agent: "Operations agent", icon: "chart-spline" },
  { channel: "#product", agent: "Research agent", icon: "search" },
] as const

const tagSteps = [
  {
    number: "01",
    title: "Spot the request",
    description: "A teammate mentions @hivy in a channel you’ve connected.",
  },
  {
    number: "02",
    title: "Bring the context",
    description:
      "Hivy reads the message, its parent post, and the replies around it.",
  },
  {
    number: "03",
    title: "Do the work",
    description:
      "The assigned agent opens or resumes the session tied to that thread.",
  },
  {
    number: "04",
    title: "Answer the thread",
    description:
      "The result comes back to Slack, right beside the original request.",
  },
] as const

const controlItems = [
  { icon: "hash", label: "Routes set by channel" },
  { icon: "bot", label: "Agents picked by your team" },
  { icon: "shield-check", label: "Public and private channels" },
  { icon: "history", label: "History attached to each thread" },
] as const

function SectionEyebrow({ children }: { children: ReactNode }) {
  return (
    <p className="text-[0.72rem] font-medium tracking-[0.12em] text-muted uppercase">
      {children}
    </p>
  )
}

function RouteDiagram() {
  return (
    <div className="overflow-hidden rounded-sm border border-border bg-surface">
      <div className="grid grid-cols-[1fr_auto_1fr] border-b border-border px-5 py-3 text-[0.68rem] font-medium tracking-[0.1em] text-muted uppercase">
        <span>Slack channel</span>
        <span className="sr-only">routes to</span>
        <span className="text-right">Assigned agent</span>
      </div>
      {routingExamples.map((route, index) => (
        <div
          key={route.channel}
          className={
            index < routingExamples.length - 1
              ? "grid grid-cols-[1fr_auto_1fr] items-center gap-4 border-b border-border px-5 py-5"
              : "grid grid-cols-[1fr_auto_1fr] items-center gap-4 px-5 py-5"
          }
        >
          <span className="inline-flex min-w-0 items-center gap-2 text-sm font-medium">
            <AppIcon icon="hash" size={14} className="shrink-0 text-muted" />
            <span className="truncate">{route.channel.slice(1)}</span>
          </span>
          <span className="flex items-center gap-1 text-muted">
            <span className="hidden h-px w-8 bg-border sm:block" />
            <AppIcon icon="arrow-right" size={14} />
            <span className="hidden h-px w-8 bg-border sm:block" />
          </span>
          <span className="inline-flex min-w-0 items-center justify-end gap-2 text-right text-sm">
            <AppIcon
              icon={route.icon}
              size={15}
              className="shrink-0 text-muted"
            />
            <span className="truncate">{route.agent}</span>
          </span>
        </div>
      ))}
    </div>
  )
}

function WorkMemoryMockup() {
  return (
    <div className="grid min-h-[500px] overflow-hidden rounded-sm border border-border bg-surface lg:grid-cols-[0.9fr_1.1fr]">
      <div className="flex flex-col justify-between border-b border-border p-7 lg:border-r lg:border-b-0 lg:p-10">
        <div>
          <div className="flex items-center gap-2 text-sm font-medium">
            <span className="flex size-8 items-center justify-center rounded-sm bg-surface-secondary">
              <AppIcon icon="brain" size={16} />
            </span>
            What the Support agent remembers
          </div>
          <div className="mt-8 space-y-3">
            <div className="rounded-sm border border-border bg-background p-4">
              <p className="text-[0.68rem] font-medium tracking-[0.08em] text-muted uppercase">
                Saved decision
              </p>
              <p className="mt-2 text-sm leading-6">
                Resolve every workspace before the import creates records.
              </p>
            </div>
            <div className="rounded-sm border border-border bg-background p-4">
              <p className="text-[0.68rem] font-medium tracking-[0.08em] text-muted uppercase">
                Useful finding
              </p>
              <p className="mt-2 text-sm leading-6">
                Multi-workspace accounts return a list, not one record.
              </p>
            </div>
          </div>
        </div>
        <p className="mt-10 text-xs leading-5 text-muted">
          The agent can carry these notes into its next Slack request.
        </p>
      </div>

      <div className="flex flex-col justify-center bg-surface-secondary p-6 md:p-10">
        <SlackMemoryPreview />
      </div>
    </div>
  )
}

export function TagLandingPage() {
  return (
    <main className="marketing-link-scope light min-h-screen bg-background text-foreground">
      <LandingHero
        titleLines={[
          "Bring @hivy into the conversation.",
          "Keep the work in Slack.",
        ]}
        description="Assign an agent to a channel, then mention @hivy, add a chosen reaction, or let the agent watch for work. It reads the conversation, does the job with its tools and knowledge, and reports back in the same thread."
        primaryAction={{ label: "Connect Slack", href: "/auth/signup" }}
        secondaryAction={{ label: "Watch @hivy work", href: "#how-it-works" }}
        placeholderLabel="Hivy working inside a Slack channel"
      />

      <section
        id="how-it-works"
        className="mx-auto mt-32 w-[calc(100%-2rem)] max-w-[1300px]"
      >
        <div className="grid items-end gap-8 lg:grid-cols-[0.85fr_1.15fr]">
          <div>
            <SectionEyebrow>Work stays with the conversation</SectionEyebrow>
            <h2 className="mt-5 max-w-[620px] text-[clamp(2rem,4vw,4rem)] leading-[0.98] font-medium tracking-[-0.055em]">
              Stop carrying Slack requests into another app.
            </h2>
          </div>
          <p className="max-w-[620px] text-[1.05rem] leading-7 text-muted lg:justify-self-end">
            The request already has names, screenshots, decisions, and replies
            around it. Mention @hivy and the assigned agent gets that context,
            works with the same tools and knowledge it uses in Hivy, then posts
            the answer where everyone can find it.
          </p>
        </div>
        <div className="mt-14 bg-surface-secondary p-4 md:p-10 lg:p-16">
          <div className="mx-auto max-w-[920px]">
            <SlackWorkspaceMockup />
          </div>
        </div>
      </section>

      <section className="mx-auto mt-40 w-[calc(100%-2rem)] max-w-[1300px]">
        <div className="grid gap-12 lg:grid-cols-[0.72fr_1.28fr] lg:items-center lg:gap-16">
          <div className="max-w-[540px]">
            <SectionEyebrow>Always-on channel watch</SectionEyebrow>
            <h2 className="mt-5 text-[clamp(2rem,3.4vw,3.35rem)] leading-none font-medium tracking-[-0.05em]">
              Some work shouldn’t wait for a mention.
            </h2>
            <p className="mt-7 text-base leading-7 text-muted">
              Give an agent a standing instruction for one channel. It can
              follow new messages, group repeated requests, take permitted
              actions, and post when the instruction calls for a response.
            </p>
            <div className="mt-9 flex items-start gap-4 border-t border-border pt-6">
              <AppIcon icon="eye" size={21} className="mt-0.5 text-muted" />
              <div>
                <p className="font-medium">
                  The channel keeps moving when nobody tags the bot.
                </p>
                <p className="mt-1 text-sm leading-6 text-muted">
                  The watch stays with the channel until an admin changes or
                  stops it.
                </p>
              </div>
            </div>
          </div>
          <SlackWatchMockup />
        </div>
      </section>

      <section className="mx-auto mt-40 w-[calc(100%-2rem)] max-w-[1300px]">
        <div className="grid gap-12 lg:grid-cols-[1.28fr_0.72fr] lg:items-center lg:gap-16">
          <SlackReactionMockup />
          <div className="max-w-[540px] lg:order-2">
            <SectionEyebrow>Reaction handoffs</SectionEyebrow>
            <h2 className="mt-5 text-[clamp(2rem,3.4vw,3.35rem)] leading-none font-medium tracking-[-0.05em]">
              Turn a reaction into a handoff.
            </h2>
            <p className="mt-7 text-base leading-7 text-muted">
              Your team already uses reactions to say “I’m looking,” “please
              review,” or “take this.” Choose one emoji and tell Hivy which
              agent should run when that reaction appears.
            </p>
            <div className="mt-9 flex items-start gap-4 border-t border-border pt-6">
              <span className="flex size-9 shrink-0 items-center justify-center rounded-sm border border-border bg-surface text-sm">
                👀
              </span>
              <div>
                <p className="font-medium">No new command to teach the team.</p>
                <p className="mt-1 text-sm leading-6 text-muted">
                  Each reaction rule can have its own channel, agent, and job.
                </p>
              </div>
            </div>
          </div>
        </div>
      </section>

      <section className="mx-auto mt-40 w-[calc(100%-2rem)] max-w-[1300px]">
        <div className="grid gap-14 lg:grid-cols-2 lg:items-center">
          <div className="max-w-[560px]">
            <SectionEyebrow>Channel ownership</SectionEyebrow>
            <h2 className="mt-5 text-[clamp(2rem,3.4vw,3.35rem)] leading-none font-medium tracking-[-0.05em]">
              Match each channel with an agent that knows the job.
            </h2>
            <p className="mt-7 text-base leading-7 text-muted">
              Product questions should reach the Product agent; incidents should
              reach Reliability. You choose the route once, and an agent can
              cover more than one channel when the work overlaps.
            </p>
            <div className="mt-9 flex items-start gap-4 border-t border-border pt-6">
              <AppIcon icon="route" size={21} className="mt-0.5 text-muted" />
              <div>
                <p className="font-medium">One route. No guessing.</p>
                <p className="mt-1 text-sm leading-6 text-muted">
                  Each channel points to one configured agent, while an existing
                  thread stays with the agent that picked it up.
                </p>
              </div>
            </div>
          </div>
          <RouteDiagram />
        </div>
      </section>

      <section className="mx-auto mt-40 w-[calc(100%-2rem)] max-w-[1300px]">
        <div className="border-y border-border">
          <div className="grid md:grid-cols-[0.72fr_repeat(4,1fr)]">
            <div className="flex flex-col justify-between border-b border-border p-6 md:border-r md:border-b-0 md:p-8">
              <SectionEyebrow>After someone mentions @hivy</SectionEyebrow>
              <p className="mt-10 text-sm leading-6 text-muted">
                One mention carries the request into the assigned agent and
                brings the result back.
              </p>
            </div>
            {tagSteps.map((step, index) => (
              <div
                key={step.number}
                className={
                  index < tagSteps.length - 1
                    ? "min-h-64 border-b border-border p-6 md:border-r md:border-b-0 md:p-8"
                    : "min-h-64 p-6 md:p-8"
                }
              >
                <span className="font-mono text-xs text-muted">
                  {step.number}
                </span>
                <h3 className="mt-14 text-lg font-medium tracking-[-0.025em]">
                  {step.title}
                </h3>
                <p className="mt-3 text-sm leading-6 text-muted">
                  {step.description}
                </p>
              </div>
            ))}
          </div>
        </div>
      </section>

      <section className="mx-auto mt-40 w-[calc(100%-2rem)] max-w-[1300px]">
        <div className="grid gap-10 lg:grid-cols-[1fr_1.55fr] lg:gap-16">
          <div className="lg:pt-8">
            <SectionEyebrow>Follow-up questions</SectionEyebrow>
            <h2 className="mt-5 text-[clamp(2rem,3.4vw,3.35rem)] leading-none font-medium tracking-[-0.05em]">
              Follow up without starting over.
            </h2>
            <p className="mt-7 max-w-[48ch] text-base leading-7 text-muted">
              The first mention connects the Slack thread to one Hivy session.
              Ask another question in that thread and the same agent continues
              with the conversation and work it already completed.
            </p>
            <div className="mt-10 flex items-center gap-3 text-sm">
              <span className="flex size-9 items-center justify-center rounded-sm border border-border bg-surface">
                <AppIcon icon="messages-square" size={17} />
              </span>
              Same thread, same agent session.
            </div>
          </div>
          <SlackThreadContinuityMockup />
        </div>
      </section>

      <section className="mx-auto mt-40 w-[calc(100%-2rem)] max-w-[1300px]">
        <div className="mb-12 grid gap-8 lg:grid-cols-2 lg:items-end">
          <div>
            <SectionEyebrow>What the agent remembers</SectionEyebrow>
            <h2 className="mt-5 text-[clamp(2rem,3.4vw,3.35rem)] leading-none font-medium tracking-[-0.05em]">
              Tomorrow’s answer remembers today’s work.
            </h2>
          </div>
          <p className="max-w-[560px] text-base leading-7 text-muted lg:justify-self-end">
            After a job, the agent can save the decisions, findings,
            conventions, and preferences worth keeping. Mention @hivy later and
            the next session can draw on those notes and the agent’s recent
            work.
          </p>
        </div>
        <WorkMemoryMockup />
      </section>

      <section className="mx-auto mt-40 w-[calc(100%-2rem)] max-w-[1300px]">
        <div className="mx-auto max-w-[820px] text-center">
          <h2 className="text-[clamp(2.35rem,4.5vw,4.5rem)] leading-[0.95] font-medium tracking-[-0.055em]">
            How teams use Hivy Tag
          </h2>
          <p className="mx-auto mt-6 max-w-[650px] text-[clamp(1rem,1.6vw,1.35rem)] leading-8 text-muted">
            Hivy agents in Slack can return the work each team needs, right in
            the thread.
          </p>
        </div>
        <div className="mt-16">
          <TeamTagUseCases />
        </div>
      </section>

      <section className="mx-auto mt-40 w-[calc(100%-2rem)] max-w-[1300px] overflow-hidden rounded-sm border border-border bg-surface">
        <div className="grid lg:grid-cols-[1.05fr_0.95fr]">
          <div className="p-7 md:p-12 lg:p-16">
            <SectionEyebrow>You choose the boundaries</SectionEyebrow>
            <h2 className="mt-5 max-w-[620px] text-[clamp(2rem,3.4vw,3.35rem)] leading-none font-medium tracking-[-0.05em]">
              @hivy only works where you’ve assigned it.
            </h2>
            <p className="mt-7 max-w-[560px] text-base leading-7 text-muted">
              Your admins choose the Slack connection, channel, team, and agent.
              Mentions in public or private channels follow that route; without
              one, @hivy won’t start an agent run.
            </p>
          </div>
          <div className="grid grid-cols-2 border-t border-border bg-surface-secondary lg:border-t-0 lg:border-l">
            {controlItems.map((item, index) => (
              <div
                key={item.label}
                className={[
                  "flex min-h-44 flex-col justify-between p-6 md:p-8",
                  index % 2 === 0 ? "border-r border-border" : "",
                  index < 2 ? "border-b border-border" : "",
                ].join(" ")}
              >
                <AppIcon icon={item.icon} size={21} className="text-muted" />
                <p className="max-w-[14ch] text-sm font-medium">{item.label}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      <section className="mx-auto flex min-h-[520px] w-[calc(100%-2rem)] max-w-[1300px] flex-col items-center justify-center text-center">
        <span className="flex size-12 items-center justify-center rounded-sm bg-surface-secondary text-foreground">
          <AppIcon icon="slack" size={24} />
        </span>
        <h2 className="mt-8 max-w-[760px] text-[clamp(2.3rem,4.5vw,4.25rem)] leading-[0.95] font-medium tracking-[-0.055em]">
          Your next handoff can stay in Slack.
        </h2>
        <p className="mt-6 max-w-[540px] text-base leading-7 text-muted">
          Create a Hivy workspace, connect Slack, and assign the agent that
          should answer in each channel.
        </p>
        <div className="mt-8 flex items-center gap-2">
          <Link href="/auth/signup">
            <Button size="sm">Connect Slack</Button>
          </Link>
          <Link href="/docs">
            <Button size="sm" variant="ghost">
              Read the setup guide
            </Button>
          </Link>
        </div>
      </section>

      <LandingFooter />
    </main>
  )
}
