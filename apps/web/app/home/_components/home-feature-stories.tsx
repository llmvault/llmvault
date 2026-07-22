"use client"

import { Button, Link } from "@heroui/react"
import { ProvisioningPreview } from "../../access-control/_components/access-control-scenes"
import { StartMethodsTabs } from "../../automations/_components/automation-previews"
import { TeamTagUseCases } from "../../tag/_components/slack-use-case-previews"
import { PlatformHighlights } from "./landing-shared"

function FeatureLink({ href, children }: { href: string; children: string }) {
  return (
    <div className="mt-8 flex justify-center">
      <Link href={href}>
        <Button size="sm">{children}</Button>
      </Link>
    </div>
  )
}

function SectionEyebrow({ children }: { children: string }) {
  return (
    <p className="text-[0.72rem] font-medium tracking-[0.12em] text-muted uppercase">
      {children}
    </p>
  )
}

export function HomeFeatureStories() {
  return (
    <div className="mt-40 space-y-40">
      <section
        aria-labelledby="home-tag-heading"
        className="mx-auto w-[calc(100%-2rem)] max-w-[1300px]"
      >
        <div className="mx-auto max-w-[820px] text-center">
          <SectionEyebrow>Hivy Tag for Slack</SectionEyebrow>
          <h2
            id="home-tag-heading"
            className="mt-5 text-[clamp(2.35rem,4.5vw,4.5rem)] leading-[0.95] font-medium tracking-[-0.055em]"
          >
            Turn a Slack message into finished work.
          </h2>
          <p className="mx-auto mt-6 max-w-[680px] text-[clamp(1rem,1.6vw,1.25rem)] leading-8 text-muted">
            Mention @hivy, react to a message, or assign an agent to watch a
            channel. Hivy brings the conversation to the right agent, gives it
            the tools and memory for the job, and posts the result back to the
            same thread.
          </p>
        </div>
        <div className="mt-16">
          <TeamTagUseCases />
        </div>
        <FeatureLink href="/tag">Explore Hivy Tag</FeatureLink>
      </section>

      <section
        aria-labelledby="home-automations-heading"
        className="mx-auto w-[calc(100%-2rem)] max-w-[1300px]"
      >
        <div className="mx-auto max-w-[820px] text-center">
          <SectionEyebrow>Automations</SectionEyebrow>
          <h2
            id="home-automations-heading"
            className="mt-5 text-[clamp(2.25rem,4.5vw,4.5rem)] leading-[0.95] font-medium tracking-[-0.055em]"
          >
            Let the next signal start the right agent.
          </h2>
          <p className="mx-auto mt-6 max-w-[680px] text-[clamp(1rem,1.6vw,1.25rem)] leading-8 text-muted">
            A pull request, Slack reaction, schedule, or webhook can open a
            fresh Hivy session automatically. Choose the trigger and owner once,
            then every matching event arrives with its request, result, and cost
            on the record.
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
                "Start an agent when work appears in GitHub, Slack, or another connected system.",
            },
            {
              icon: "calendar",
              title: "Run on a schedule",
              description:
                "Give recurring reviews, checks, and reports a reliable cadence without another reminder.",
            },
            {
              icon: "globe",
              title: "Accept a webhook",
              description:
                "Turn an authenticated request from your product into a session owned by the right team.",
            },
          ]}
        />
        <FeatureLink href="/automations">Explore automations</FeatureLink>
      </section>

      <section
        aria-labelledby="home-access-heading"
        className="mx-auto w-[calc(100%-2rem)] max-w-[1300px]"
      >
        <div className="mx-auto max-w-[980px] text-center">
          <SectionEyebrow>Team access control</SectionEyebrow>
          <h2
            id="home-access-heading"
            className="mt-5 text-[clamp(2.2rem,4.2vw,4.25rem)] leading-[0.96] font-medium tracking-[-0.055em] text-balance"
          >
            Give every agent what the job needs. Nothing outside it.
          </h2>
          <p className="mx-auto mt-6 max-w-[680px] text-base leading-7 text-muted">
            Set connections, knowledge sources, and published skills once at the
            team level. Every assigned agent inherits that boundary, and Hivy
            checks it again whenever work starts.
          </p>
        </div>
        <div className="mx-auto mt-14 max-w-[980px] bg-surface-secondary p-4 md:p-9 lg:p-14">
          <ProvisioningPreview />
        </div>
        <PlatformHighlights
          className="mx-auto max-w-[980px] border-t"
          items={[
            {
              icon: "plug",
              title: "Connections",
              description:
                "Choose the company accounts a team’s agents may use without exposing the rest of your workspace.",
            },
            {
              icon: "database",
              title: "Knowledge sources",
              description:
                "Limit search to the repositories, channels, pages, and sites approved for that team.",
            },
            {
              icon: "sparkles",
              title: "Published skills",
              description:
                "Give approved procedures to the agents that need them while keeping every other team separate.",
            },
          ]}
        />
        <FeatureLink href="/access-control">Explore access control</FeatureLink>
      </section>
    </div>
  )
}
