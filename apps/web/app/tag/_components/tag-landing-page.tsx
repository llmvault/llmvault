import type { ReactNode } from "react"
import { Button, Link } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import {
  LandingFooter,
  LandingHero,
} from "../../home/_components/landing-shared"
import {
  SlackMemoryPreview,
  SlackReactionMockup,
  SlackThreadContinuityMockup,
  SlackWatchMockup,
  SlackWorkspaceMockup,
} from "./slack-previews"

const routingExamples = [
  { channel: "#product-support", agent: "Support agent", icon: "headset" },
  { channel: "#incidents", agent: "Reliability agent", icon: "radar" },
  { channel: "#revenue-ops", agent: "Operations agent", icon: "chart-spline" },
  { channel: "#product", agent: "Research agent", icon: "search" },
] as const

const requestExamples = [
  {
    prompt: "@hivy summarize the decision and list the open questions.",
    result: "Turn a long discussion into a short handoff for the channel.",
  },
  {
    prompt: "@hivy reproduce this bug and tell us where it starts.",
    result: "Send the assigned agent from the report into its connected tools.",
  },
  {
    prompt:
      "@hivy compare these requests with what customers asked last month.",
    result: "Bring the agent's earlier work into the current conversation.",
  },
  {
    prompt: "@hivy take this from here and report back in the thread.",
    result: "Keep the request, work, and answer in one shared place.",
  },
] as const

const tagSteps = [
  {
    number: "01",
    title: "Receive the mention",
    description:
      "Hivy accepts a tag in a configured public or private workspace channel.",
  },
  {
    number: "02",
    title: "Read the thread",
    description:
      "The agent gets the message, the parent post, and the thread around it.",
  },
  {
    number: "03",
    title: "Run the agent",
    description:
      "Hivy starts or resumes the session tied to that Slack thread.",
  },
  {
    number: "04",
    title: "Reply in place",
    description:
      "The final answer returns to the same thread for everyone to see.",
  },
] as const

const controlItems = [
  { icon: "hash", label: "Per-channel routing" },
  { icon: "bot", label: "Named agent ownership" },
  { icon: "shield-check", label: "Public or private channels" },
  { icon: "history", label: "Thread session history" },
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
            Support agent memory
          </div>
          <div className="mt-8 space-y-3">
            <div className="rounded-sm border border-border bg-background p-4">
              <p className="text-[0.68rem] font-medium tracking-[0.08em] text-muted uppercase">
                Decision
              </p>
              <p className="mt-2 text-sm leading-6">
                Import jobs must resolve every workspace before creating
                records.
              </p>
            </div>
            <div className="rounded-sm border border-border bg-background p-4">
              <p className="text-[0.68rem] font-medium tracking-[0.08em] text-muted uppercase">
                Finding
              </p>
              <p className="mt-2 text-sm leading-6">
                Multi-workspace accounts need a list-aware lookup path.
              </p>
            </div>
          </div>
        </div>
        <p className="mt-10 text-xs leading-5 text-muted">
          Example memory entries, shown as interface placeholders.
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
          "Put Hivy to work from Slack.",
          "Tag it in. Keep the thread moving.",
        ]}
        description="Assign a Hivy agent to each Slack channel. Tag @hivy in a message or thread, and that agent can read the conversation, do the work, and reply where your team is already talking."
        primaryAction={{ label: "Start for free", href: "/auth/signup" }}
        secondaryAction={{ label: "See how it works", href: "#how-it-works" }}
        placeholderLabel="Hivy Tag in Slack product screenshot"
      />

      <section
        id="how-it-works"
        className="mx-auto mt-32 w-[calc(100%-2rem)] max-w-[1300px]"
      >
        <div className="grid items-end gap-8 lg:grid-cols-[0.85fr_1.15fr]">
          <div>
            <SectionEyebrow>Hivy in Slack</SectionEyebrow>
            <h2 className="mt-5 max-w-[620px] text-[clamp(2rem,4vw,4rem)] leading-[0.98] font-medium tracking-[-0.055em]">
              A Slack message becomes a working session.
            </h2>
          </div>
          <p className="max-w-[620px] text-[1.05rem] leading-7 text-muted lg:justify-self-end">
            Hivy pulls in the message and its thread, hands it to the agent
            assigned to that channel, and posts the answer back in place. The
            Hivy session behind it can use the same model, tools, and knowledge
            as a session started in the app.
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
            <SectionEyebrow>Channel watch</SectionEyebrow>
            <h2 className="mt-5 text-[clamp(2rem,3.4vw,3.35rem)] leading-none font-medium tracking-[-0.05em]">
              Put a channel on watch.
            </h2>
            <p className="mt-7 text-base leading-7 text-muted">
              Assign an agent to watch a Slack channel and give it standing
              instructions. It follows new messages, groups repeated work, takes
              allowed actions, and posts when the instruction calls for it.
            </p>
            <div className="mt-9 flex items-start gap-4 border-t border-border pt-6">
              <AppIcon icon="eye" size={21} className="mt-0.5 text-muted" />
              <div>
                <p className="font-medium">
                  No one has to remember the mention.
                </p>
                <p className="mt-1 text-sm leading-6 text-muted">
                  The assigned agent keeps the standing job in that channel
                  until an admin changes or stops it.
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
            <SectionEyebrow>Reaction triggers</SectionEyebrow>
            <h2 className="mt-5 text-[clamp(2rem,3.4vw,3.35rem)] leading-none font-medium tracking-[-0.05em]">
              One emoji can start the work.
            </h2>
            <p className="mt-7 text-base leading-7 text-muted">
              Choose a channel, an emoji, the agent, and its instructions. When
              a teammate adds that reaction, Hivy reads the message and thread,
              starts the assigned agent, then replies in the same conversation.
            </p>
            <div className="mt-9 flex items-start gap-4 border-t border-border pt-6">
              <span className="flex size-9 shrink-0 items-center justify-center rounded-sm border border-border bg-surface text-sm">
                👀
              </span>
              <div>
                <p className="font-medium">
                  Use the signal your team already knows.
                </p>
                <p className="mt-1 text-sm leading-6 text-muted">
                  Set a different emoji and instruction for each Slack reaction
                  automation.
                </p>
              </div>
            </div>
          </div>
        </div>
      </section>

      <section className="mx-auto mt-40 w-[calc(100%-2rem)] max-w-[1300px]">
        <div className="grid gap-14 lg:grid-cols-2 lg:items-center">
          <div className="max-w-[560px]">
            <SectionEyebrow>Channel routing</SectionEyebrow>
            <h2 className="mt-5 text-[clamp(2rem,3.4vw,3.35rem)] leading-none font-medium tracking-[-0.05em]">
              Give every channel the right agent.
            </h2>
            <p className="mt-7 text-base leading-7 text-muted">
              Pick one agent for a configured Slack channel. That same agent can
              cover other channels too, so its job follows your team’s actual
              workflow instead of a separate inbox.
            </p>
            <div className="mt-9 flex items-start gap-4 border-t border-border pt-6">
              <AppIcon icon="route" size={21} className="mt-0.5 text-muted" />
              <div>
                <p className="font-medium">The route stays predictable.</p>
                <p className="mt-1 text-sm leading-6 text-muted">
                  A channel points to one configured agent. Existing threads
                  stay with the agent that started the work.
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
              <SectionEyebrow>From tag to answer</SectionEyebrow>
              <p className="mt-10 text-sm leading-6 text-muted">
                One mention opens a shared path from Slack to the assigned Hivy
                agent.
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
            <SectionEyebrow>Thread continuity</SectionEyebrow>
            <h2 className="mt-5 text-[clamp(2rem,3.4vw,3.35rem)] leading-none font-medium tracking-[-0.05em]">
              Keep talking. Hivy keeps the thread.
            </h2>
            <p className="mt-7 max-w-[48ch] text-base leading-7 text-muted">
              The first tag binds the Slack thread to one Hivy session. Follow
              up in that thread and the same agent continues with the same
              conversation instead of starting over.
            </p>
            <div className="mt-10 flex items-center gap-3 text-sm">
              <span className="flex size-9 items-center justify-center rounded-sm border border-border bg-surface">
                <AppIcon icon="messages-square" size={17} />
              </span>
              One Slack thread. One active Hivy session.
            </div>
          </div>
          <SlackThreadContinuityMockup />
        </div>
      </section>

      <section className="mx-auto mt-40 w-[calc(100%-2rem)] max-w-[1300px]">
        <div className="mb-12 grid gap-8 lg:grid-cols-2 lg:items-end">
          <div>
            <SectionEyebrow>Agent memory</SectionEyebrow>
            <h2 className="mt-5 text-[clamp(2rem,3.4vw,3.35rem)] leading-none font-medium tracking-[-0.05em]">
              The next tag starts with what the agent learned.
            </h2>
          </div>
          <p className="max-w-[560px] text-base leading-7 text-muted lg:justify-self-end">
            Hivy reflects on completed work and stores useful decisions,
            findings, conventions, and preferences for the agent. Future
            sessions can also draw on its recent work, including sessions that
            began in Slack.
          </p>
        </div>
        <WorkMemoryMockup />
      </section>

      <section className="mx-auto mt-40 w-[calc(100%-2rem)] max-w-[1300px]">
        <SectionEyebrow>What to hand off</SectionEyebrow>
        <div className="mt-5 grid gap-10 lg:grid-cols-[0.75fr_1.25fr]">
          <h2 className="max-w-[520px] text-[clamp(2rem,3.4vw,3.35rem)] leading-none font-medium tracking-[-0.05em]">
            If the work starts in Slack, let it stay there.
          </h2>
          <div className="border-t border-border">
            {requestExamples.map((example) => (
              <div
                key={example.prompt}
                className="grid gap-4 border-b border-border py-7 md:grid-cols-[1fr_0.8fr] md:gap-10"
              >
                <p className="text-[1.05rem] leading-6 font-medium tracking-[-0.02em]">
                  {example.prompt}
                </p>
                <p className="text-sm leading-6 text-muted">{example.result}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      <section className="mx-auto mt-40 w-[calc(100%-2rem)] max-w-[1300px] overflow-hidden rounded-sm border border-border bg-surface">
        <div className="grid lg:grid-cols-[1.05fr_0.95fr]">
          <div className="p-7 md:p-12 lg:p-16">
            <SectionEyebrow>Controlled by your team</SectionEyebrow>
            <h2 className="mt-5 max-w-[620px] text-[clamp(2rem,3.4vw,3.35rem)] leading-none font-medium tracking-[-0.05em]">
              Hivy answers where you put it.
            </h2>
            <p className="mt-7 max-w-[560px] text-base leading-7 text-muted">
              Configure the connection, channel, team, and agent in Hivy.
              Mentions in public or private workspace channels use that route.
              If a channel has no route, Hivy does not start an agent run there.
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
          Your next agent request can start in Slack.
        </h2>
        <p className="mt-6 max-w-[540px] text-base leading-7 text-muted">
          Create your Hivy workspace, connect Slack, and choose the agent for
          each channel your team wants to use.
        </p>
        <div className="mt-8 flex items-center gap-2">
          <Link href="/auth/signup">
            <Button size="sm">Start for free</Button>
          </Link>
          <Link href="/docs">
            <Button size="sm" variant="ghost">
              Read the docs
            </Button>
          </Link>
        </div>
      </section>

      <LandingFooter />
    </main>
  )
}
