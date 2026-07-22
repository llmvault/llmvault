import Link from "next/link"
import type { ReactNode } from "react"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

const INSTALL_STEPS = [
  {
    number: "01",
    title: "Find the right specialist",
    description:
      "Search by name or browse categories until you find the narrowest agent that matches the outcome you need.",
  },
  {
    number: "02",
    title: "Review its requirements",
    description:
      "Check the agent’s purpose and required connections before choosing a team; Hivy refuses the installation when a requirement is missing.",
  },
  {
    number: "03",
    title: "Install it for a team",
    description:
      "Pick a team you belong to, and Hivy creates a separate copy for that team's work.",
  },
  {
    number: "04",
    title: "Test the installed copy",
    description:
      "Run one representative task for the team before changing the model, sandbox, or optional connections.",
  },
]

export function AgentCatalog() {
  return (
    <div className="mt-10 text-base leading-7">
      <p className="max-w-2xl text-muted">
        Catalog agents arrive with a defined job and the settings needed to do
        it. Install one when it already fits the work instead of rebuilding the
        same setup yourself.
      </p>

      <DocsMediaPlaceholder
        type="video"
        title="Choose and install your first agent"
        description="Record the catalog search, the agent’s requirements, and the team installation from start to finish."
        className="mt-12"
      />

      <section aria-labelledby="choose-and-install" className="mt-16">
        <h2
          id="choose-and-install"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          Choose and install an agent
        </h2>
        <p className="mt-3 max-w-2xl text-muted">
          Open <strong className="font-medium text-foreground">Agents</strong>{" "}
          from the workspace sidebar, where Hivy keeps catalog entries beside
          the agents your workspace created.
        </p>

        <ol className="mt-8 divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
          {INSTALL_STEPS.map((step) => (
            <li
              key={step.title}
              className="grid gap-3 p-5 sm:grid-cols-[3rem_1fr] sm:gap-4"
            >
              <span className="text-xs font-semibold tracking-[0.12em] text-muted">
                {step.number}
              </span>
              <div>
                <h3 className="font-semibold text-foreground">{step.title}</h3>
                <p className="mt-1 text-sm leading-6 text-muted">
                  {step.description}
                </p>
              </div>
            </li>
          ))}
        </ol>
      </section>

      <DocsMediaPlaceholder
        type="image"
        title="A catalog agent’s team installation screen"
        description="Include the agent details, required connections, and per-team Install controls in one readable frame."
        className="mt-12"
      />

      <div className="mt-16 space-y-14 border-t border-border pt-14">
        <DocSection title="Each team gets its own agent">
          <p>
            Hivy creates a separate copy for the selected team, limited to that
            team&apos;s capabilities. If another team needs the same specialist,
            install another copy; its access and settings remain separate.
          </p>
        </DocSection>

        <DocSection title="Hivy checks required connections first">
          <p>
            Before installation, Hivy checks whether the selected team has every
            connection the agent requires. If one is missing, the page names it
            and no agent gets created.
          </p>
          <p className="mt-3">
            The installed agent receives the team&apos;s connections, though you
            can switch off an optional connection for this agent without
            affecting its teammates. Connections listed as required are locked
            on because the catalog agent depends on them.
          </p>
        </DocSection>

        <DocSection title="Tune it for the work and budget">
          <p>
            After installation, set the model and sandbox for the work this
            agent will handle most often, then remove optional connections it
            won&apos;t use. These choices affect cost and available compute.
          </p>
        </DocSection>

        <section
          aria-labelledby="catalog-or-custom"
          className="rounded-xl border border-border bg-surface-secondary p-6"
        >
          <h2
            id="catalog-or-custom"
            className="text-lg font-semibold tracking-tight text-foreground"
          >
            Catalog agent or custom agent?
          </h2>
          <p className="mt-2 max-w-xl text-sm leading-6 text-muted">
            Pick a catalog agent when its listed job matches yours. If the role
            needs different instructions, tools, or delegation rules, create a
            custom agent instead.
          </p>
          <Link
            href="/docs/agents/configure-an-agent"
            className="mt-4 inline-flex rounded-sm text-sm font-medium text-foreground underline decoration-border underline-offset-4 transition-colors hover:text-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus"
          >
            Create and configure an agent
          </Link>
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
  const id =
    "catalog-" +
    title.toLowerCase().replaceAll(/[?,]/g, "").replaceAll(" ", "-")

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
