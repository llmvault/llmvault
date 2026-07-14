import Link from "next/link"
import type { ReactNode } from "react"
import { AppIcon } from "@/components/icon"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

export function RunFirstAgent() {
  return (
    <div className="mt-10 text-base leading-7">
      <p className="max-w-2xl text-muted">
        You’ll ask a specialist to research customer-acquisition ideas and save
        them in a sheet. Before writing the prompt, choose an agent and model
        that fit the job.
      </p>

      <div className="mt-14 space-y-14 border-t border-border pt-14">
        <DocStep number="1" title="Choose the agent for the job">
          <p>
            Select <strong>New chat</strong> in the workspace, then choose the
            team that owns the work. Pick a channel for the session; the agent
            menu will contain only agents from that team.
          </p>
          <p className="mt-3">
            Match the agent’s job and tools to the result. A specialist won’t
            waste model usage working out a role it doesn’t have.
          </p>
          <p className="mt-3">
            If none of the available agents fit,{" "}
            <Link
              href="/docs/agents/configure-an-agent"
              className="rounded-sm font-medium text-foreground underline decoration-border underline-offset-4 transition-colors hover:text-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus"
            >
              create an agent for the job
            </Link>
            .
          </p>
        </DocStep>

        <DocStep number="2" title="Use a cost-efficient model">
          <p>
            Use DeepSeek V4 Flash with Low reasoning for this research task. The
            example session cost less than $0.10; extra tool calls or a costlier
            model will raise that amount.
          </p>
          <DocsMediaPlaceholder
            className="mt-6"
            type="image"
            title="Choose DeepSeek V4 Flash"
            description="Capture the model picker at 100% zoom with DeepSeek V4 Flash and Low reasoning visible. Crop close enough to read every model name."
          />
        </DocStep>

        <DocStep number="3" title="Ask for a clear outcome">
          <p>
            State the result you want and name the fields it must contain. Leave
            the tool choices to the agent instead of writing a click-by-click
            procedure.
          </p>
          <div className="mt-6 rounded-xl border border-border bg-surface-secondary p-5 text-sm leading-6 text-foreground">
            <p className="text-xs font-semibold tracking-[0.12em] text-muted uppercase">
              Example task
            </p>
            <p className="mt-3">
              Find 10 customer-acquisition strategies that an early-stage SaaS
              company can try. Save them in a sheet with these fields: strategy,
              description, implementation steps, estimated cost, timeframe,
              difficulty, impact, and source URL.
            </p>
          </div>
        </DocStep>

        <DocStep number="4" title="Review and steer the session">
          <p>
            Once the agent finishes, check its response and the sheet. Ask for
            corrections in the same session; “add three more,” for example,
            continues with the context already there.
          </p>
          <p className="mt-3">
            Sheets are channel-scoped databases that hold information beyond a
            single session. People can edit the rows directly, and agents in
            that channel can work with the data later.
          </p>
          <DocsMediaPlaceholder
            className="mt-6"
            type="image"
            title="Review the session and its sheet"
            description="Capture a completed research session beside its populated sheet at 100% zoom. Include the follow-up composer and session cost without making the table text too small to read."
          />
        </DocStep>

        <DocStep number="5" title="Build on the result">
          <p>
            To turn the sheet into an app, start a session with{" "}
            <strong>Ricky - App builder</strong>. Tell Ricky which sheet to use
            and what the app should let people do.
          </p>
          <p className="mt-3">
            Apps can read from a Hivy sheet, your database, or an external
            service. A larger build takes more time and model tokens, so check
            the session cost before asking for another round of work.
          </p>
          <DocsMediaPlaceholder
            className="mt-6"
            type="video"
            title="Turn the sheet into an app with Ricky"
            description="Record a 45-second clip at 100% zoom. Select Ricky - App builder, ask it to use the existing sheet, open the finished app, and end on the session cost."
          />
        </DocStep>

        <section
          aria-labelledby="first-session-checklist"
          className="rounded-xl border border-border bg-surface-secondary p-6"
        >
          <h2
            id="first-session-checklist"
            className="text-lg font-semibold tracking-tight text-foreground"
          >
            Before you start
          </h2>
          <ul className="mt-4 space-y-3 text-sm text-muted">
            {[
              "Match the agent to the job.",
              "For this task, use DeepSeek V4 Flash with Low reasoning.",
              "Name the result and its required fields.",
              "Check the sheet, then make corrections in the same session.",
              "Choose Ricky - App builder if you want an app, and watch the session cost as it works.",
            ].map((item) => (
              <li key={item} className="flex gap-3">
                <AppIcon
                  icon="check"
                  className="mt-1 h-4 w-4 shrink-0 text-accent"
                />
                <span>{item}</span>
              </li>
            ))}
          </ul>
        </section>
      </div>
    </div>
  )
}

function DocStep({
  number,
  title,
  children,
}: {
  number: string
  title: string
  children: ReactNode
}) {
  const id = `step-${number}-${title.toLowerCase().replaceAll(" ", "-")}`

  return (
    <section aria-labelledby={id}>
      <p className="text-xs font-semibold tracking-[0.12em] text-muted uppercase">
        Step {number}
      </p>
      <h2
        id={id}
        className="mt-2 text-xl font-semibold tracking-tight text-foreground"
      >
        {title}
      </h2>
      <div className="mt-3 max-w-2xl text-muted">{children}</div>
    </section>
  )
}
