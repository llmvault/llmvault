import type { ReactNode } from "react"
import { Button, Link } from "@heroui/react"
import {
  LandingFooter,
  LandingHero,
  PlatformHighlights,
} from "../../home/_components/landing-shared"
import {
  DatabaseBrowserPreview,
  LiveAgentUpdatePreview,
} from "./sheets-previews"

function SectionEyebrow({ children }: { children: ReactNode }) {
  return (
    <p className="text-[0.72rem] font-medium tracking-[0.12em] text-muted uppercase">
      {children}
    </p>
  )
}

export function SheetsLandingPage() {
  return (
    <main className="marketing-link-scope light min-h-screen bg-background text-foreground">
      <LandingHero
        titleLines={[
          "Give Hivy agents a database they can read and write.",
          "Keep the state between sessions.",
        ]}
        description="Store the leads, accounts, tickets, and other records your team works with. People and agents can query rows, update fields, and return to the same data in the next session."
        primaryAction={{
          label: "Create free workspace",
          href: "/auth/signup",
        }}
        secondaryAction={{
          label: "See a Sheet in use",
          href: "#shared-records",
        }}
        placeholderLabel="A shared Hivy Sheet with account records"
      />

      <section
        id="shared-records"
        className="mx-auto mt-32 w-[calc(100%-2rem)] max-w-[1300px]"
      >
        <div className="grid items-end gap-8 lg:grid-cols-[0.82fr_1.18fr]">
          <div>
            <SectionEyebrow>Structured data for agent work</SectionEyebrow>
            <h2 className="mt-5 max-w-[650px] text-[clamp(2rem,4vw,4rem)] leading-[0.98] font-medium tracking-[-0.055em]">
              Give agents records they can act on.
            </h2>
          </div>
          <p className="max-w-[610px] text-[1.05rem] leading-7 text-muted lg:justify-self-end">
            Keep rows in the same workspace as the agents that use them. Start
            empty, import a CSV, then let people and agents search, filter, and
            update the same structured records.
          </p>
        </div>
        <div className="mt-14 bg-surface-secondary p-4 md:p-10 lg:p-16">
          <DatabaseBrowserPreview />
        </div>
        <PlatformHighlights
          items={[
            {
              icon: "table",
              title: "Rows agents can act on",
              description:
                "Store accounts, tickets, tasks, or any other operational record in a database built for the team doing the work.",
            },
            {
              icon: "list-filter",
              title: "Search and filter before changing",
              description:
                "Find the exact records that match the job instead of loading an entire CSV into every prompt.",
            },
            {
              icon: "columns-3",
              title: "Typed fields keep shape",
              description:
                "Named fields keep owners, statuses, dates, links, and next steps in the right place.",
            },
          ]}
        />
      </section>

      <section className="mx-auto mt-40 w-[calc(100%-2rem)] max-w-[1300px]">
        <div className="grid gap-12 lg:grid-cols-[0.68fr_1.32fr] lg:items-end">
          <div className="max-w-[520px]">
            <SectionEyebrow>State that survives the run</SectionEyebrow>
            <h2 className="mt-5 text-[clamp(2rem,3.5vw,3.55rem)] leading-none font-medium tracking-[-0.05em]">
              Every session starts from the latest records.
            </h2>
          </div>
          <p className="max-w-[620px] text-base leading-7 text-muted lg:justify-self-end">
            Ask an agent to turn notes into records, update fields, or find what
            needs attention. Its results write back to the Sheet, so the next
            person or agent continues from the new state.
          </p>
        </div>
        <div className="mt-12">
          <LiveAgentUpdatePreview />
        </div>
        <PlatformHighlights
          items={[
            {
              icon: "database",
              title: "State between sessions",
              description:
                "A finished session writes useful results back, and the next session begins with those changes in place.",
            },
            {
              icon: "bot",
              title: "Agents read and write rows",
              description:
                "Turn notes into records, update matching rows, and report exactly what changed in the same conversation.",
            },
            {
              icon: "users",
              title: "One database for the team",
              description:
                "People and assigned agents work from the same Sheet instead of passing temporary tables between chats.",
            },
          ]}
        />
      </section>

      <section
        id="contact"
        className="mx-auto mt-40 flex min-h-[520px] w-[calc(100%-2rem)] max-w-[1300px] flex-col items-center justify-center border-t border-border text-center"
      >
        <SectionEyebrow>Bring one real list</SectionEyebrow>
        <h2 className="mt-6 max-w-[850px] text-[clamp(2.3rem,4.5vw,4.5rem)] leading-[0.96] font-medium tracking-[-0.06em]">
          Give your first agent a database it can come back to.
        </h2>
        <p className="mt-7 max-w-[560px] text-base leading-7 text-muted">
          Create a free workspace, import a CSV or start a Sheet, then let your
          team and its agents work from the same records.
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
