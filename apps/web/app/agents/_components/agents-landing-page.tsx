import type { ReactNode } from "react"
import { Button, Link } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import {
  LandingFooter,
  LandingHero,
  PlatformHighlights,
} from "../../home/_components/landing-shared"
import { AgentCatalogExplorer } from "./agent-catalog-explorer"

function SectionEyebrow({ children }: { children: ReactNode }) {
  return (
    <p className="text-[0.72rem] font-medium tracking-[0.12em] text-muted uppercase">
      {children}
    </p>
  )
}

const formSections = [
  ["Role", "Product research"],
  ["Model", "DeepSeek V4 Flash"],
  ["Instructions", "Outcome, method, limits"],
  ["Tools", "Search, files, browser, shell"],
  ["Team access", "Product"],
  ["Specialists", "2 available"],
] as const

function AgentBuilderScene() {
  return (
    <div className="overflow-hidden rounded-sm border border-border bg-surface shadow-xs">
      <div className="flex h-12 items-center justify-between border-b border-border px-5">
        <div className="flex items-center gap-2 text-sm font-medium">
          <span className="flex size-7 items-center justify-center rounded-sm bg-accent-soft">
            <AppIcon icon="bot" size={15} />
          </span>
          New agent
        </div>
        <Button size="sm">Create agent</Button>
      </div>
      <div className="grid md:grid-cols-[0.34fr_0.66fr]">
        <div className="border-b border-border bg-surface-secondary p-4 md:border-r md:border-b-0">
          {formSections.map(([label, value], index) => (
            <div
              key={label}
              className={`rounded-sm px-3 py-3 ${index === 2 ? "bg-surface shadow-xs" : ""}`}
            >
              <div className="flex items-center justify-between gap-3">
                <p className="text-xs font-medium">{label}</p>
                {index < 2 ? (
                  <AppIcon
                    icon="circle-check"
                    size={13}
                    className="text-success"
                  />
                ) : null}
              </div>
              <p className="mt-1 truncate text-[0.68rem] text-muted">{value}</p>
            </div>
          ))}
        </div>
        <div className="p-5 sm:p-7">
          <div className="flex items-start justify-between gap-4">
            <div>
              <p className="text-sm font-semibold">Instructions</p>
              <p className="mt-1 text-xs text-muted">
                Tell the agent what to deliver and when to ask for approval.
              </p>
            </div>
            <span className="rounded-sm bg-surface-secondary px-2 py-1 text-[0.65rem] text-muted">
              Product
            </span>
          </div>
          <div className="mt-5 min-h-64 rounded-sm border border-border bg-background p-4 text-xs leading-6 text-foreground sm:p-5">
            <p className="font-medium">
              Your job is to answer product questions with customer evidence.
            </p>
            <p className="mt-3 text-muted">
              Search approved sources, group repeated requests, compare them
              with current roadmap work, and return a brief with links.
            </p>
            <p className="mt-3 text-muted">
              Don’t edit the roadmap. If the evidence conflicts, explain the
              disagreement instead of choosing a side.
            </p>
            <div className="mt-6 flex flex-wrap gap-2">
              {[
                "Customer evidence",
                "Cited brief",
                "Roadmap context",
                "Approval required",
              ].map((item) => (
                <span
                  key={item}
                  className="rounded-sm border border-border bg-surface px-2 py-1 text-[0.68rem] text-muted"
                >
                  {item}
                </span>
              ))}
            </div>
          </div>
          <div className="mt-4 grid gap-2 text-xs sm:grid-cols-3">
            {[
              ["Last test", "Completed"],
              ["Tools used", "Search · Files · Browser"],
              ["Session", "Open trace"],
            ].map(([label, value]) => (
              <div
                key={label}
                className="rounded-sm bg-surface-secondary px-3 py-3"
              >
                <p className="text-[0.68rem] text-muted">{label}</p>
                <p className="mt-1 truncate font-medium">{value}</p>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

export function AgentsLandingPage() {
  return (
    <main className="marketing-link-scope min-h-screen bg-background text-foreground">
      <LandingHero
        titleLines={[
          "Build a Hivy agent for every repeatable job.",
          "Give it tools, context, and limits.",
        ]}
        description="Start with a ready-made specialist or define the role yourself. Choose its model, tools, knowledge, team access, and sandbox, then inspect every session it runs."
        placeholderLabel="Agent catalog and custom agent setup"
      />

      <section
        id="catalog"
        className="mx-auto mt-32 w-[calc(100%-2rem)] max-w-[1300px]"
      >
        <div className="grid items-end gap-8 lg:grid-cols-[0.8fr_1.2fr]">
          <div>
            <SectionEyebrow>Start from the job</SectionEyebrow>
            <h2 className="mt-5 max-w-[620px] text-[clamp(2rem,4vw,4rem)] leading-[0.98] font-medium tracking-[-0.055em]">
              Install a specialist. Or define the role yourself.
            </h2>
          </div>
          <p className="max-w-[620px] text-[1.05rem] leading-7 text-muted lg:justify-self-end">
            Choose an agent for support, product, engineering, or operations.
            Install it for one team, then adapt the model, instructions, tools,
            and sandbox to the work it owns.
          </p>
        </div>
        <div className="mt-14 bg-surface-secondary p-4 md:p-10 lg:p-16">
          <div className="mx-auto max-w-[980px]">
            <AgentCatalogExplorer />
          </div>
        </div>
        <PlatformHighlights
          items={[
            {
              icon: "layout-grid",
              title: "Ready-made specialists",
              description:
                "Start with an agent built for a real team job, then install it only where that work happens.",
            },
            {
              icon: "file-text",
              title: "A role written in plain language",
              description:
                "Define the outcome, method, limits, and approval points without turning the job into a prompt maze.",
            },
            {
              icon: "cpu",
              title: "Your model and sandbox",
              description:
                "Choose the model and compute size that fit the job instead of forcing every agent onto one setup.",
            },
          ]}
        />
      </section>

      <section className="mx-auto mt-40 w-[calc(100%-2rem)] max-w-[1300px]">
        <div className="grid gap-12 lg:grid-cols-[0.68fr_1.32fr] lg:items-center lg:gap-16">
          <div className="max-w-[520px]">
            <SectionEyebrow>From setup to real work</SectionEyebrow>
            <h2 className="mt-5 text-[clamp(2rem,3.4vw,3.35rem)] leading-none font-medium tracking-[-0.05em]">
              Run real work without losing control of the agent.
            </h2>
            <p className="mt-7 text-base leading-7 text-muted">
              The role keeps its instructions, tools, knowledge, specialists,
              team, and sandbox together. Each session preserves the request,
              tool use, answer, and cost for your team to inspect.
            </p>
          </div>
          <AgentBuilderScene />
        </div>
        <PlatformHighlights
          items={[
            {
              icon: "plug",
              title: "Tools and knowledge attached",
              description:
                "Give the role the connections, files, sources, and specialists it needs to finish the job.",
            },
            {
              icon: "activity",
              title: "Every session inspectable",
              description:
                "Open the request, tool use, answer, duration, and real cost after the agent works.",
            },
            {
              icon: "refresh-cw",
              title: "A role that improves",
              description:
                "Turn a repeatable correction into a better instruction instead of fixing the same output again.",
            },
          ]}
        />
      </section>

      <section className="mx-auto flex min-h-[540px] w-[calc(100%-2rem)] max-w-[1300px] flex-col items-center justify-center text-center">
        <span className="flex size-12 items-center justify-center rounded-sm bg-surface-secondary">
          <AppIcon icon="bot" size={24} />
        </span>
        <h2 className="mt-8 max-w-[800px] text-[clamp(2.3rem,4.5vw,4.25rem)] leading-[0.95] font-medium tracking-[-0.055em]">
          Build the first agent your team can keep using.
        </h2>
        <p className="mt-6 max-w-[570px] text-base leading-7 text-muted">
          Open a free workspace, install a specialist or describe the role, then
          test it on a real request.
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
