import type { ReactNode } from "react"
import { Button, Link } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import {
  LandingFooter,
  LandingHero,
  PlatformHighlights,
} from "../../home/_components/landing-shared"
import {
  ProvisioningPreview,
  TeamBoundaryPreview,
} from "./access-control-scenes"

function SectionEyebrow({ children }: { children: ReactNode }) {
  return (
    <p className="text-[0.72rem] font-medium tracking-[0.12em] text-muted uppercase">
      {children}
    </p>
  )
}

export function AccessControlLandingPage() {
  return (
    <main className="marketing-link-scope light min-h-screen bg-background text-foreground">
      <LandingHero
        titleLines={[
          "Govern every Hivy agent from one workspace.",
          "Set access before work starts.",
        ]}
        description="Group people and agents by team, choose the connections, knowledge, and skills each team can use, and let Hivy enforce the same boundary in every session."
        primaryAction={{
          label: "Create free workspace",
          href: "/auth/signup",
        }}
        secondaryAction={{
          label: "See team access",
          href: "#team-boundary",
        }}
        placeholderLabel="Team access settings in Hivy"
      />

      <section
        id="team-boundary"
        className="mx-auto mt-32 w-[calc(100%-2rem)] max-w-[1300px]"
      >
        <div className="grid gap-12 lg:grid-cols-[0.76fr_1.24fr] lg:items-center lg:gap-16">
          <div className="max-w-[540px]">
            <SectionEyebrow>Roles, teams, and grants</SectionEyebrow>
            <h2 className="mt-5 text-[clamp(2rem,3.6vw,3.7rem)] leading-[0.98] font-medium tracking-[-0.05em]">
              Control who can do what, team by team.
            </h2>
            <p className="mt-7 text-base leading-7 text-muted">
              Owners and admins manage the workspace. Members work through the
              teams they belong to. Each team carries the agents and resources
              its job calls for.
            </p>
            <div className="mt-9 flex items-start gap-4 border-t border-border pt-6">
              <AppIcon icon="network" size={20} className="mt-0.5 text-muted" />
              <div>
                <p className="text-sm font-medium">
                  Review the full access path.
                </p>
                <p className="mt-1 text-sm leading-6 text-muted">
                  Follow a person to their team, its agents, and the resources
                  those agents may use.
                </p>
              </div>
            </div>
          </div>
          <TeamBoundaryPreview />
        </div>
        <PlatformHighlights
          items={[
            {
              icon: "settings",
              title: "Workspace roles",
              description:
                "Owners and admins manage the workspace while members work inside the teams they have joined.",
            },
            {
              icon: "users",
              title: "Teams define the working boundary",
              description:
                "Group people and agents around a real function such as Product, Support, or Operations.",
            },
            {
              icon: "network",
              title: "Resources granted once",
              description:
                "Attach the connections, knowledge sources, and skills that the team’s work requires.",
            },
          ]}
        />
      </section>

      <section className="mx-auto mt-40 w-[calc(100%-2rem)] max-w-[1300px]">
        <div className="mx-auto max-w-[820px] text-center">
          <SectionEyebrow>One boundary for every session</SectionEyebrow>
          <h2 className="mt-5 text-[clamp(2.2rem,4.2vw,4.25rem)] leading-[0.96] font-medium tracking-[-0.055em]">
            Give agents what the job needs. Nothing outside it.
          </h2>
          <p className="mx-auto mt-6 max-w-[650px] text-base leading-7 text-muted">
            Switch on the exact accounts, indexed sources, and published skills
            a team can use. Hivy resolves those grants again whenever an agent
            starts work.
          </p>
        </div>
        <div className="mx-auto mt-14 max-w-[980px] bg-surface-secondary p-4 md:p-9 lg:p-14">
          <ProvisioningPreview />
        </div>
        <PlatformHighlights
          items={[
            {
              icon: "plug",
              title: "Connections",
              description:
                "Choose the connected accounts an agent may act through instead of exposing every provider in the workspace.",
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
                "Make approved procedures available to the agents that need them while keeping other teams separate.",
            },
          ]}
        />
      </section>

      <section className="mx-auto flex min-h-[520px] w-[calc(100%-2rem)] max-w-[1300px] flex-col items-center justify-center text-center">
        <span className="flex size-12 items-center justify-center rounded-sm bg-accent-soft">
          <AppIcon icon="shield-check" size={23} />
        </span>
        <h2 className="mt-8 max-w-[760px] text-[clamp(2.3rem,4.5vw,4.25rem)] leading-[0.95] font-medium tracking-[-0.055em]">
          Set the boundary before your first agent runs.
        </h2>
        <p className="mt-6 max-w-[560px] text-base leading-7 text-muted">
          Create a free workspace, add a team, and assign only the connections,
          knowledge, and skills its work calls for.
        </p>
        <div className="mt-8">
          <Link href="/auth/signup">
            <Button size="sm">Create free workspace</Button>
          </Link>
        </div>
      </section>

      <LandingFooter />
    </main>
  )
}
