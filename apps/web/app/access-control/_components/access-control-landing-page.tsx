import type { ReactNode } from "react"
import { Button, Link } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import {
  LandingFooter,
  LandingHero,
  PlatformHighlights,
} from "../../home/_components/landing-shared"
import { ProvisioningPreview } from "./access-control-scenes"

function SectionEyebrow({ children }: { children: ReactNode }) {
  return (
    <p className="text-[0.72rem] font-medium tracking-[0.12em] text-muted uppercase">
      {children}
    </p>
  )
}

export function AccessControlLandingPage() {
  return (
    <main className="marketing-link-scope min-h-screen bg-background text-foreground">
      <LandingHero
        titleLines={[
          "Govern every Hivy agent from one workspace.",
          "Set access before work starts.",
        ]}
        description="Group people and agents by team, choose the connections, knowledge, and skills each team can use, and let Hivy enforce the same boundary in every session."
        placeholderLabel="Team access settings in Hivy"
      />

      <section
        id="team-boundary"
        className="mx-auto mt-32 w-[calc(100%-2rem)] max-w-[1300px] border-t border-border pt-16 md:pt-20"
      >
        <div className="grid gap-14 lg:grid-cols-[0.82fr_1.18fr] lg:gap-24">
          <div className="max-w-[560px]">
            <SectionEyebrow>Roles, teams, and grants</SectionEyebrow>
            <h2 className="mt-5 text-[clamp(2rem,3.6vw,3.7rem)] leading-[0.98] font-medium tracking-[-0.05em]">
              Control who can do what, team by team.
            </h2>
            <p className="mt-7 text-base leading-7 text-muted">
              Owners and admins manage the workspace. Members work through the
              teams they belong to. Each team carries the agents and resources
              its job calls for.
            </p>
          </div>
          <div className="border-t border-border">
            {[
              {
                title: "Workspace roles",
                description:
                  "Owners and admins set up the workspace. Members work inside the teams they join.",
              },
              {
                title: "Teams set the boundary",
                description:
                  "Group people and agents around a function such as Product, Support, or Operations.",
              },
              {
                title: "Resources follow the team",
                description:
                  "Grant the connections, knowledge sources, and skills that team needs, once.",
              },
            ].map((item) => (
              <article
                key={item.title}
                className="grid gap-3 border-b border-border py-7 sm:grid-cols-[0.72fr_1.28fr] sm:gap-8"
              >
                <h3 className="text-base font-medium tracking-[-0.02em]">
                  {item.title}
                </h3>
                <p className="max-w-[46ch] text-sm leading-6 text-muted">
                  {item.description}
                </p>
              </article>
            ))}
          </div>
        </div>
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
        <div data-testid="resource-grant-highlights" className="pt-10">
          <PlatformHighlights
            className="border-t"
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
        </div>
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
            <Button size="sm">Start for free</Button>
          </Link>
        </div>
      </section>

      <LandingFooter />
    </main>
  )
}
