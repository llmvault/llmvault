import type { ReactNode } from "react"
import { Button, Link } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import {
  LandingFooter,
  LandingHero,
  PlatformHighlights,
} from "../../home/_components/landing-shared"
import { CitedAnswerScene, SourceSetupScene } from "./knowledge-scenes"

function SectionEyebrow({ children }: { children: ReactNode }) {
  return (
    <p className="text-[0.72rem] font-medium tracking-[0.12em] text-muted uppercase">
      {children}
    </p>
  )
}

export function KnowledgeLandingPage() {
  return (
    <main className="marketing-link-scope light min-h-screen bg-background text-foreground">
      <LandingHero
        titleLines={[
          "Give Hivy agents memory of your company’s work.",
          "Get every answer with its source.",
        ]}
        description="Connect selected Slack channels, GitHub repositories, Notion pages, Linear teams, and websites. Hivy indexes the work, keeps access scoped by team, and returns answers with citations."
        primaryAction={{
          label: "Create free workspace",
          href: "/auth/signup",
        }}
        secondaryAction={{
          label: "See connected sources",
          href: "#source-setup",
        }}
        placeholderLabel="Connected sources and an answer with citations"
      />

      <section
        id="source-setup"
        className="mx-auto mt-32 w-[calc(100%-2rem)] max-w-[1300px]"
      >
        <div className="grid items-end gap-8 lg:grid-cols-[0.9fr_1.1fr]">
          <div>
            <SectionEyebrow>One memory for every assigned agent</SectionEyebrow>
            <h2 className="mt-5 max-w-[680px] text-[clamp(2rem,4vw,4rem)] leading-[0.98] font-medium tracking-[-0.055em]">
              Give agents company context they can actually use.
            </h2>
          </div>
          <p className="max-w-[620px] text-[1.05rem] leading-7 text-muted lg:justify-self-end">
            Choose the repositories, channels, pages, and site sections that
            matter. Assign each source to the right teams, and every agent on
            those teams can search the same approved context.
          </p>
        </div>
        <div className="mt-14 bg-surface-secondary p-4 md:p-8 lg:p-12">
          <SourceSetupScene />
        </div>
        <PlatformHighlights
          items={[
            {
              icon: "scan-search",
              title: "Sync the sources that matter",
              description:
                "Select exact repositories, channels, pages, teams, or site sections instead of indexing everything.",
            },
            {
              icon: "users",
              title: "Share one memory with a team",
              description:
                "Every assigned agent searches the same approved knowledge, so context stays consistent from one session to the next.",
            },
            {
              icon: "shield-check",
              title: "Keep access attached",
              description:
                "Knowledge grants follow the team. Agents do not surface sources their team cannot use.",
            },
          ]}
        />
      </section>

      <section className="mx-auto mt-40 w-[calc(100%-2rem)] max-w-[1300px]">
        <div className="mb-12 grid gap-8 lg:grid-cols-[0.9fr_1.1fr] lg:items-end">
          <div>
            <SectionEyebrow>Grounded answers, fresh context</SectionEyebrow>
            <h2 className="mt-5 max-w-[700px] text-[clamp(2rem,3.7vw,3.7rem)] leading-[0.98] font-medium tracking-[-0.05em]">
              Answers stay grounded as your sources change.
            </h2>
          </div>
          <p className="max-w-[580px] text-base leading-7 text-muted lg:justify-self-end">
            Hivy indexes source changes and keeps existing documents searchable
            while updates run. Every answer names the source, links back to it,
            and shows the evidence used.
          </p>
        </div>
        <div className="bg-accent-soft p-4 md:p-8 lg:p-12">
          <CitedAnswerScene />
        </div>
        <PlatformHighlights
          items={[
            {
              icon: "text-search",
              title: "Answers grounded in your work",
              description:
                "Hivy searches approved sources and cites the document, thread, issue, or page behind each claim.",
            },
            {
              icon: "external-link",
              title: "Evidence one click away",
              description:
                "Open the matching source and excerpt without repeating the search or trusting an unsupported summary.",
            },
            {
              icon: "refresh-cw",
              title: "Changed sources re-index",
              description:
                "Hivy picks up source changes while the last successful index remains available to agents.",
            },
          ]}
        />
      </section>

      <section className="mx-auto flex min-h-[540px] w-[calc(100%-2rem)] max-w-[1300px] flex-col items-center justify-center text-center">
        <span className="flex size-12 items-center justify-center rounded-sm bg-surface-secondary text-foreground">
          <AppIcon icon="database" size={23} />
        </span>
        <h2 className="mt-8 max-w-[820px] text-[clamp(2.3rem,4.5vw,4.25rem)] leading-[0.95] font-medium tracking-[-0.055em]">
          Give your first agent company memory.
        </h2>
        <p className="mt-6 max-w-[560px] text-base leading-7 text-muted">
          Create a free workspace, connect one source, and decide which team can
          search it.
        </p>
        <div className="mt-8 flex items-center gap-2">
          <Link href="/auth/signup">
            <Button size="sm">Create free workspace</Button>
          </Link>
        </div>
      </section>

      <LandingFooter />
    </main>
  )
}
