import Link from "next/link"
import type { ReactNode } from "react"
import { AppIcon } from "@/components/icon"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

const APP_WORKFLOW = [
  {
    number: "01",
    title: "Describe the outcome",
    description:
      "Name the person using the app and the one job its first version must handle well.",
  },
  {
    number: "02",
    title: "Choose the data",
    description:
      "Point Ricky to one Hivy Sheet, then say which rows the app should show or change.",
  },
  {
    number: "03",
    title: "Review a preview",
    description:
      "Try the main task in the temporary preview. Keep corrections in the same session so Ricky can edit the current build.",
  },
  {
    number: "04",
    title: "Approve deployment",
    description:
      "Tell Ricky to publish only after the preview works. Hivy checks the deployment before Ricky shares the live URL.",
  },
]

export function AgentBuiltApps() {
  return (
    <div className="mt-10 text-base leading-7">
      <p className="max-w-2xl text-muted">
        Ask Ricky for the interface your work calls for, rather than forcing the
        job into a generic form. Each app binds to one Hivy Sheet; you review
        and publish it from the same session.
      </p>

      <DocsMediaPlaceholder
        type="video"
        title="Build an app with Ricky from request to deployment"
        description="Record a new session with Ricky - App builder. Give Ricky the app brief and data source, test its preview, request one change, then approve the production deployment. Finish on the running app and its row on the Apps page."
        className="mt-12"
      />

      <section
        aria-labelledby="build-the-smallest-useful-app"
        className="mt-16"
      >
        <h2
          id="build-the-smallest-useful-app"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          Build the smallest useful app first
        </h2>
        <p className="mt-3 max-w-2xl text-muted">
          Pick one job that someone can test end to end. It might be a lead
          review screen, an inventory editor, an approval queue, or a research
          dashboard. A small first version exposes bad assumptions quickly; a
          sprawling brief hides them until late in the build.
        </p>

        <ol className="mt-8 divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
          {APP_WORKFLOW.map((item) => (
            <li key={item.title} className="p-5">
              <span className="text-xs font-semibold tracking-[0.12em] text-muted uppercase">
                {item.number}
              </span>
              <h3 className="mt-4 font-semibold text-foreground">
                {item.title}
              </h3>
              <p className="mt-2 text-sm leading-6 text-muted">
                {item.description}
              </p>
            </li>
          ))}
        </ol>
      </section>

      <DocsMediaPlaceholder
        type="image"
        title="Ricky beside a running app preview"
        description="Frame the Ricky - App builder session beside Browser, with the preview large enough to read. Include demo data, the preview link, the completed build step, and the follow-up box; crop out personal data and secrets."
        className="mt-12"
      />

      <div className="mt-16 space-y-14 border-t border-border pt-14">
        <DocSection title="Choose Ricky, App builder">
          <p>
            Start a new session and select{" "}
            <strong className="text-foreground">Ricky - App builder</strong>.
            Its app and Sheets tools cover the whole job, including previews,
            production releases, logs, and rollbacks.
          </p>
          <p className="mt-3">
            Say who uses the app and what they need to finish. For example,
            &quot;Build a lead review app for the sales team. Let people filter
            by status, open a company record, edit the next step, and see its
            owner.&quot;
          </p>
          <DocLink href="/w">Start a session with Ricky</DocLink>
        </DocSection>

        <DocSection title="Tell Ricky where the data lives">
          <p>
            Name the Hivy Sheet that should back the app and which rows people
            may change. One app binds to one Sheet, and it can read and update
            rows without changing that Sheet&apos;s schema.
          </p>
          <p className="mt-3">
            If the data lives in Hivy, name the existing Sheet or ask Ricky to
            make one first. Ricky reads its pages and field types before writing
            the app, while the Sheet remains available to agents and teammates
            outside the new interface.
          </p>
          <DocLink href="/docs/sheets-and-apps/sheets">
            Learn how Sheets work
          </DocLink>
        </DocSection>

        <DocSection title="Review the preview before you publish">
          <p>
            Ricky checks a temporary preview inside its session before sharing
            it. Open Browser and complete the main task with believable demo
            data. The preview disappears when the builder session ends.
          </p>
          <p className="mt-3">
            Keep follow-up work in that session because Ricky can edit the
            current app instead of reconstructing it. Be literal: &quot;Put
            overdue items first; add an owner filter and ask for confirmation
            before deleting a row.&quot;
          </p>
        </DocSection>

        <DocSection title="Approve each production deployment">
          <p>
            A preview isn&apos;t a production app. Ricky needs your explicit
            approval each time it publishes or republishes. Once you approve,
            Hivy stores a new version and deploys it; Ricky checks the running
            app before returning its live URL.
          </p>
          <p className="mt-3">
            Ricky writes notes for every release, which makes old versions easy
            to identify. If the release breaks something, ask it to read the
            production logs and restore the last good version before attempting
            another fix.
          </p>
        </DocSection>

        <DocSection title="Open apps from one place">
          <p>
            Select <strong className="text-foreground">Apps</strong> in the
            workspace sidebar. Hivy groups the list by team and lets you search
            names or descriptions. Open a result in the right panel, or send it
            to a separate browser tab when you need more room.
          </p>
          <p className="mt-3">
            App access follows its team. A person must be able to use that team
            before Hivy lists the app or issues a launch session.
          </p>
          <DocLink href="/w/apps">Open Apps</DocLink>
        </DocSection>

        <section
          aria-labelledby="keep-secrets-out-of-the-request"
          className="rounded-xl border border-border bg-surface-secondary p-6"
        >
          <div className="flex items-start gap-3">
            <span className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-border bg-surface text-muted">
              <AppIcon icon="shield-check" className="h-4 w-4" />
            </span>
            <div>
              <h2
                id="keep-secrets-out-of-the-request"
                className="text-lg font-semibold tracking-tight text-foreground"
              >
                Connect data without pasting secrets into the session
              </h2>
              <p className="mt-2 max-w-xl text-sm leading-6 text-muted">
                Don&apos;t paste a password, token, or private connection string
                into the request. An app uses its bound Sheet; other agent work
                should receive credentials through Connections or encrypted team
                environment variables.
              </p>
            </div>
          </div>
        </section>

        <DocsMediaPlaceholder
          type="image"
          title="The Apps page grouped by team"
          description="Frame the Apps page with demo entries under two teams and keep Search visible. Open one app in the right panel, make its interface readable, and remove personal data from the frame."
          bleed={false}
        />
      </div>
    </div>
  )
}

function DocSection({
  title,
  children,
}: {
  title: string
  children: ReactNode
}) {
  const id = "apps-" + title.toLowerCase().replaceAll(" ", "-")

  return (
    <section aria-labelledby={id}>
      <h2
        id={id}
        className="text-xl font-semibold tracking-tight text-foreground"
      >
        {title}
      </h2>
      <div className="mt-3 max-w-2xl text-muted">{children}</div>
    </section>
  )
}

function DocLink({ href, children }: { href: string; children: ReactNode }) {
  return (
    <Link
      href={href}
      className="mt-5 inline-flex items-center gap-2 rounded-sm text-sm font-medium text-foreground transition-colors hover:text-accent focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-focus"
    >
      {children}
      <AppIcon icon="arrow-right" className="h-4 w-4" />
    </Link>
  )
}
