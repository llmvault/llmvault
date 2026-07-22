import type { ReactNode } from "react"
import { Button, Link } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import {
  LandingFooter,
  LandingHero,
  PlatformHighlights,
} from "../../home/_components/landing-shared"
import { RunHistoryMockup, StartMethodsTabs } from "./automation-previews"

function SectionEyebrow({ children }: { children: ReactNode }) {
  return (
    <p className="text-[0.72rem] font-medium tracking-[0.12em] text-muted uppercase">
      {children}
    </p>
  )
}

export function AutomationsLandingPage() {
  return (
    <main className="marketing-link-scope min-h-screen bg-background text-foreground">
      <LandingHero
        titleLines={[
          "Run Hivy agents when the work arrives.",
          "No one has to press go.",
        ]}
        description="Start the right agent from a pull request, Slack reaction, schedule, or webhook. Every match opens a Hivy session with the request, result, follow-ups, and cost together."
        placeholderLabel="Automation trigger and completed agent session"
      />

      <section
        id="how-it-starts"
        className="mx-auto mt-32 w-[calc(100%-2rem)] max-w-[1300px]"
      >
        <div className="mx-auto max-w-[800px] text-center">
          <SectionEyebrow>Triggers for real work</SectionEyebrow>
          <h2 className="mt-5 text-[clamp(2.25rem,4.5vw,4.5rem)] leading-[0.95] font-medium tracking-[-0.055em]">
            Put any agent on the signal that starts its job.
          </h2>
          <p className="mx-auto mt-6 max-w-[650px] text-[clamp(1rem,1.6vw,1.25rem)] leading-8 text-muted">
            Choose the trigger, assign its team and agent, then describe the
            finished result. Hivy turns each match into a fresh session without
            asking someone to copy the request into chat.
          </p>
        </div>
        <div className="mt-14">
          <StartMethodsTabs />
        </div>
        <PlatformHighlights
          items={[
            {
              icon: "plug",
              title: "React to connected tools",
              description:
                "Start work from events such as a new pull request or a reaction inside a Slack channel.",
            },
            {
              icon: "calendar",
              title: "Run on a schedule",
              description:
                "Give recurring reviews, digests, and checks a cadence instead of another reminder for a teammate.",
            },
            {
              icon: "globe",
              title: "Accept a webhook",
              description:
                "Turn an authenticated request from your app into a session owned by the right agent and team.",
            },
          ]}
        />
      </section>

      <section className="mx-auto mt-40 w-[calc(100%-2rem)] max-w-[1300px]">
        <div className="grid gap-8 lg:grid-cols-[0.92fr_1.08fr] lg:items-end">
          <div>
            <SectionEyebrow>Every run on the record</SectionEyebrow>
            <h2 className="mt-5 max-w-[650px] text-[clamp(2rem,3.4vw,3.35rem)] leading-none font-medium tracking-[-0.05em]">
              See what ran, what it cost, and what happened.
            </h2>
          </div>
          <p className="max-w-[560px] text-base leading-7 text-muted lg:justify-self-end">
            Every automation creates a normal Hivy session. Open the request,
            answer, status, duration, and cost, then follow up with the same
            agent when the result needs another pass.
          </p>
        </div>
        <div className="mt-12">
          <RunHistoryMockup />
        </div>
        <PlatformHighlights
          items={[
            {
              icon: "activity",
              title: "Watch the run as a session",
              description:
                "The trigger, request, agent response, and current status live together instead of behind a job ID.",
            },
            {
              icon: "history",
              title: "Keep the run history",
              description:
                "Return to completed and failed work with the duration and cost still attached.",
            },
            {
              icon: "message-square",
              title: "Follow up on the result",
              description:
                "Ask the same agent to revise or continue from the automated session without rebuilding the request.",
            },
          ]}
        />
      </section>

      <section className="mx-auto flex min-h-[540px] w-[calc(100%-2rem)] max-w-[1300px] flex-col items-center justify-center text-center">
        <span className="flex size-12 items-center justify-center rounded-sm bg-surface-secondary text-foreground">
          <AppIcon icon="workflow" size={24} />
        </span>
        <h2 className="mt-8 max-w-[800px] text-[clamp(2.3rem,4.5vw,4.25rem)] leading-[0.95] font-medium tracking-[-0.055em]">
          Put one repeatable job on autopilot.
        </h2>
        <p className="mt-6 max-w-[560px] text-base leading-7 text-muted">
          Create your free Hivy workspace, choose the signal, and assign the
          agent that should own every run.
        </p>
        <div className="mt-8 flex items-center gap-2">
          <Link href="/auth/signup">
            <Button size="sm">Start for free</Button>
          </Link>
        </div>
      </section>

      <LandingFooter />
    </main>
  )
}
