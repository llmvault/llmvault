import Link from "next/link"
import type { ReactNode } from "react"
import { AppIcon } from "@/components/icon"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

const SKILL_PARTS = [
  ["Name and slug", "A recognizable name and stable identifier for the skill."],
  [
    "Description",
    "A plain account of when the skill applies, so an agent can choose it for the right job.",
  ],
  [
    "Instructions",
    "The process, constraints, and reference material the agent should follow after loading it.",
  ],
] as const

export function Skills() {
  return (
    <div className="mt-10 text-base leading-7">
      <p className="max-w-2xl text-muted">
        A skill is a reusable set of instructions, not a connection. It teaches
        an agent how to do a job with the tools and access it already has.
      </p>

      <section aria-labelledby="skill-shape" className="mt-14">
        <h2
          id="skill-shape"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          Write a skill an agent can recognize
        </h2>
        <dl className="mt-6 divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
          {SKILL_PARTS.map(([term, description]) => (
            <div
              key={term}
              className="grid gap-1 px-4 py-4 sm:grid-cols-[9rem_1fr] sm:gap-5"
            >
              <dt className="text-sm font-semibold text-foreground">{term}</dt>
              <dd className="text-sm leading-6 text-muted">{description}</dd>
            </div>
          ))}
        </dl>
      </section>

      <DocsMediaPlaceholder
        type="video"
        title="Create and grant a reusable skill"
        description="Create a team skill with a concrete description and short instructions, run a matching agent task, then show the agent loading the skill in the session."
        className="mt-12"
      />

      <div className="mt-16 space-y-14 border-t border-border pt-14">
        <DocSection title="Choose its scope when you create it">
          <p>
            A team skill is available to that team automatically. Team members
            can create and manage skills for teams they belong to.
          </p>
          <p className="mt-3">
            Admins can create a workspace-wide skill instead. It stays out of a
            team until an admin grants it from that team&apos;s settings.
          </p>
        </DocSection>

        <DocsMediaPlaceholder
          type="image"
          title="Skill scope and instructions"
          description="Capture Settings > Skills with the add-skill dialog open. Show the name, slug, description, scope, and instruction fields with safe demo content."
        />

        <DocSection title="Describe the trigger, then write the method">
          <p>
            Put selection language in the description: name the request, output,
            or condition that should cause an agent to load the skill. Keep the
            instruction body focused on what to do after that choice.
          </p>
          <p className="mt-3">
            Don&apos;t store tokens, passwords, or account credentials in skill
            text. Use a connection or team environment variable for secret
            values.
          </p>
        </DocSection>

        <DocSection title="Let tools and skills do different jobs">
          <p>
            A skill can tell an agent how your team reviews an incident. The
            connection supplies the Datadog or Slack tools; the skill does not
            grant either account.
          </p>
          <DocLink href="/docs/connections-and-skills/how-access-works">
            Read how connections grant tools
          </DocLink>
        </DocSection>

        <section
          aria-labelledby="archive-skill"
          className="rounded-xl border border-border bg-surface-secondary p-6"
        >
          <AppIcon icon="archive" className="h-4 w-4 text-accent" />
          <h2
            id="archive-skill"
            className="mt-4 text-lg font-semibold tracking-tight text-foreground"
          >
            Archive skills agents should stop using
          </h2>
          <p className="mt-2 max-w-xl text-sm leading-6 text-muted">
            Archiving removes the skill from agent access. Update the skill when
            its method changes; archive it when the job itself no longer exists.
          </p>
        </section>
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
  const id = "skills-" + title.toLowerCase().replaceAll(" ", "-")
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
      className="mt-4 inline-flex rounded-sm text-sm font-medium text-foreground underline decoration-border underline-offset-4 transition-colors hover:text-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus"
    >
      {children}
    </Link>
  )
}
