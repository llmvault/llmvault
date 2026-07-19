import { Button, Link, Separator, Skeleton } from "@heroui/react"
import { AppIcon } from "@/components/icon"

export const pillars = [
  {
    icon: "plug",
    title: "Integrate",
    description:
      "Hivy's catalog of 1,000+ integrations your agents act through.",
  },
  {
    icon: "database",
    title: "Context",
    description:
      "Your data, stored semantically in Hivy as the memory your agents reason over.",
  },
  {
    icon: "workflow",
    title: "Build",
    description:
      "Compose agent logic in the visual builder, or just describe it to Hivy.",
  },
  {
    icon: "monitor",
    title: "Monitor",
    description:
      "See inside every run in Hivy: live traces, logs, and real cost.",
  },
] as const

const placeholderLogos = [
  {
    name: "Aster",
    icon: "sparkles",
    color: "text-[oklch(58%_0.2_29)]",
    background: "bg-[oklch(94%_0.035_29)]",
  },
  {
    name: "Northline",
    icon: "workflow",
    color: "text-[oklch(54%_0.16_255)]",
    background: "bg-[oklch(94%_0.035_255)]",
  },
  {
    name: "Mosaic",
    icon: "layout-grid",
    color: "text-[oklch(57%_0.16_145)]",
    background: "bg-[oklch(94%_0.035_145)]",
  },
  {
    name: "Orbit",
    icon: "circle",
    color: "text-[oklch(57%_0.2_315)]",
    background: "bg-[oklch(94%_0.035_315)]",
  },
  {
    name: "Tandem",
    icon: "focus",
    color: "text-[oklch(58%_0.17_65)]",
    background: "bg-[oklch(95%_0.04_65)]",
  },
  {
    name: "Current",
    icon: "activity",
    color: "text-[oklch(55%_0.17_205)]",
    background: "bg-[oklch(94%_0.035_205)]",
  },
] as const

export const features = [
  {
    label: "Integrate",
    title: "Connect the tools your work runs on.",
    description:
      "Plug in 1,000+ integrations like Slack, HubSpot, Salesforce, and Notion, so Hivy agents act across the stack you already use.",
    action: "Explore integrations",
    icon: "plug",
    placeholder: "Integrations product screenshot",
  },
  {
    label: "Context",
    title: "Give Hivy data it can reason over.",
    description:
      "Hivy stores your data in tables, files, and knowledge bases, the semantic memory agents read to ground every answer.",
    action: undefined,
    icon: "database",
    placeholder: "Knowledge base product screenshot",
  },
  {
    label: "Build",
    title: "Build agents that solve real problems.",
    description:
      "Wire blocks, models, and integrations into agent logic in Hivy's visual builder, from one agent to many working in parallel.",
    action: "Explore the workflow builder",
    icon: "workflow",
    placeholder: "Workflow builder product screenshot",
  },
  {
    label: "Monitor",
    title: "Watch every run, end to end.",
    description:
      "Hivy traces each run block by block, with full logs and the real cost.",
    action: undefined,
    icon: "monitor",
    placeholder: "Agent run logs product screenshot",
  },
] as const

const footerGroups = [
  {
    title: "Product",
    links: [
      "Enterprise",
      "Chat",
      "Workflows",
      "Knowledge Base",
      "Tables",
      "Files",
      "Logs",
      "Scheduled Tasks",
      "MCP",
      "API",
      "Self Hosting",
      "Status",
    ],
  },
  {
    title: "Resources",
    links: ["Blog", "Docs", "Library", "Careers", "Changelog", "Contact"],
  },
  {
    title: "Compare",
    links: [
      "All Comparisons",
      "n8n",
      "Zapier",
      "Make",
      "Gumloop",
      "Workato",
      "Retool",
      "Pipedream",
      "OpenAI AgentKit",
      "Tines",
    ],
  },
  {
    title: "Integrations",
    links: [
      "All Integrations",
      "Slack",
      "GitHub",
      "Gmail",
      "Notion",
      "Salesforce",
      "Jira",
      "Linear",
      "Supabase",
      "Stripe",
    ],
  },
  {
    title: "Models",
    links: [
      "All Models",
      "OpenAI",
      "Anthropic",
      "Google",
      "DeepSeek",
      "xAI",
      "Cerebras",
      "Groq",
      "Sakana AI",
    ],
  },
  {
    title: "Socials",
    links: ["X (Twitter)", "LinkedIn", "Slack", "GitHub"],
  },
  {
    title: "Legal",
    links: ["Terms of Service", "Privacy Policy"],
  },
] as const

export function ProductPlaceholder({
  label,
  className = "",
}: {
  label: string
  className?: string
}) {
  return (
    <div
      role="img"
      aria-label={`${label}, placeholder`}
      className={`relative flex h-full min-h-72 overflow-hidden bg-surface-secondary md:min-h-0 ${className}`}
    >
      <div className="absolute inset-x-[9%] top-[10%] bottom-[8%] overflow-hidden rounded-sm border border-border bg-surface shadow-xs">
        <div className="flex h-10 items-center gap-2 border-b border-border px-4">
          <span className="h-2 w-2 rounded-full bg-muted/45" />
          <span className="h-2 w-16 rounded-full bg-muted/20" />
        </div>
        <div className="grid h-[calc(100%-2.5rem)] grid-cols-[26%_1fr]">
          <div className="space-y-3 border-r border-border p-4">
            <Skeleton className="h-2.5 w-4/5 rounded-sm" />
            <Skeleton className="h-2.5 w-3/5 rounded-sm" />
            <Skeleton className="h-7 w-full rounded-sm" />
            <Skeleton className="h-2.5 w-2/3 rounded-sm" />
            <Skeleton className="h-2.5 w-4/5 rounded-sm" />
          </div>
          <div className="space-y-4 p-5">
            <Skeleton className="h-5 w-2/5 rounded-sm" />
            <Skeleton className="h-9 w-full rounded-sm" />
            <Skeleton className="h-2.5 w-5/6 rounded-sm" />
            <Skeleton className="h-2.5 w-3/4 rounded-sm" />
            <Skeleton className="h-2.5 w-4/6 rounded-sm" />
          </div>
        </div>
      </div>
      <span className="sr-only">{label}</span>
    </div>
  )
}

export function FeatureCopy({
  title,
  description,
  action,
  className = "",
}: {
  title: string
  description: string
  action?: string
  className?: string
}) {
  return (
    <div className={`flex flex-col justify-center ${className}`}>
      <h3 className="max-w-sm text-[clamp(1.3rem,1.7vw,1.75rem)] leading-tight font-medium tracking-[-0.025em]">
        {title}
      </h3>
      <p className="mt-4 max-w-[38ch] text-[0.95rem] leading-6 text-muted">
        {description}
      </p>
      {action ? (
        <Link
          href="#"
          className="mt-6 inline-flex w-fit items-center gap-2 text-sm font-medium text-foreground"
        >
          {action}
          <AppIcon icon="arrow-right" size={15} />
        </Link>
      ) : null}
    </div>
  )
}

function LandingHeader() {
  return (
    <header className="mx-auto flex h-16 w-[calc(100%-2rem)] max-w-[1300px] items-center justify-between">
      <div className="flex items-center gap-7">
        <Link
          href="/home"
          aria-label="Hivy home"
          className="text-[1.05rem] font-semibold tracking-[-0.05em] text-foreground"
        >
          hivy
        </Link>
        <nav
          className="hidden items-center gap-7 text-[0.82rem] md:flex"
          aria-label="Main navigation"
        >
          <Link href="#product" className="gap-1.5 text-foreground/80">
            Platform <AppIcon icon="chevron-down" size={12} />
          </Link>
          <Link href="#capabilities" className="gap-1.5 text-foreground/80">
            Solutions <AppIcon icon="chevron-down" size={12} />
          </Link>
          <Link href="#resources" className="gap-1.5 text-foreground/80">
            Resources <AppIcon icon="chevron-down" size={12} />
          </Link>
          <Link href="#" className="text-foreground/80">
            Pricing
          </Link>
          <Link
            href="https://github.com/usehivy/hivy"
            className="gap-1.5 text-foreground/80"
          >
            <AppIcon icon="github" size={14} /> 29.2k
          </Link>
        </nav>
      </div>

      <div className="flex items-center gap-2">
        <Link href="/auth/login" className="hidden sm:inline-flex">
          <Button size="sm" variant="ghost">
            Log in
          </Button>
        </Link>
        <Link href="#contact" className="hidden sm:inline-flex">
          <Button size="sm" variant="secondary">
            Contact sales
          </Button>
        </Link>
        <Link href="/auth/signup">
          <Button size="sm">Sign up</Button>
        </Link>
      </div>
    </header>
  )
}

export function LandingHero() {
  return (
    <>
      <LandingHeader />
      <section className="mx-auto w-[calc(100%-2rem)] max-w-[1300px] pt-28">
        <div>
          <h1 className="max-w-[760px] text-[clamp(3rem,4.3vw,3.65rem)] leading-[0.98] font-medium tracking-[-0.055em] text-balance">
            Hivy is the AI workspace
            <br className="hidden sm:block" /> for building AI agents.
          </h1>
          <p className="mt-7 max-w-[630px] text-[1rem] leading-6 text-muted">
            Open source, with 1,000+ integrations and every major LLM. Build,
            deploy, and manage agents visually, conversationally, or with code.
          </p>
          <div className="mt-8 flex items-center gap-2">
            <Link href="#contact">
              <Button size="sm">Request a demo</Button>
            </Link>
            <Link href="/auth/signup">
              <Button size="sm" variant="ghost">
                Sign up
              </Button>
            </Link>
          </div>
        </div>

        <div
          id="product"
          className="mt-16 aspect-[1.82/1] overflow-hidden rounded-sm bg-surface-tertiary p-[7%]"
        >
          <ProductPlaceholder label="Main Hivy workspace product screenshot" />
        </div>
      </section>
    </>
  )
}

export function TrustStrip() {
  return (
    <section
      aria-labelledby="trusted-heading"
      className="mx-auto mt-16 w-[calc(100%-2rem)] max-w-[1120px]"
    >
      <h2
        id="trusted-heading"
        className="text-center text-xs font-normal text-muted"
      >
        Trusted by technical teams at
      </h2>
      <div className="mt-8 grid grid-cols-2 items-center gap-x-8 gap-y-7 sm:grid-cols-3 lg:grid-cols-6">
        {placeholderLogos.map((logo) => (
          <div
            key={logo.name}
            aria-label={`${logo.name} placeholder logo`}
            className="flex items-center justify-center gap-2.5"
          >
            <span
              className={`flex size-9 items-center justify-center rounded-lg ${logo.background} ${logo.color}`}
            >
              <AppIcon icon={logo.icon} size={20} strokeWidth={2} />
            </span>
            <span
              className={`text-base font-semibold tracking-[-0.025em] ${logo.color}`}
            >
              {logo.name}
            </span>
          </div>
        ))}
      </div>
    </section>
  )
}

export function WorkflowPrompt({ className = "" }: { className?: string }) {
  return (
    <section
      className={`mx-auto w-[calc(100%-2rem)] max-w-[1300px] overflow-hidden rounded-sm border border-border bg-surface ${className}`}
    >
      <div className="grid min-h-[640px] md:grid-cols-[2fr_1fr]">
        <div className="relative flex items-center justify-center bg-surface-secondary p-8">
          <div className="w-full max-w-[315px] rounded-lg border border-border bg-surface shadow-sm">
            <div className="flex items-center gap-3 border-b border-border px-4 py-3">
              <AppIcon icon="github" size={25} />
              <span className="text-xl font-medium">GitHub</span>
            </div>
            <div className="flex items-center justify-between px-4 py-3 text-sm text-muted">
              <span>Trigger</span>
              <span className="text-foreground">PR opened</span>
            </div>
          </div>
        </div>
        <div className="flex flex-col justify-center px-9 py-14 md:px-12">
          <div className="mb-10 flex items-center gap-6 text-muted/70">
            <AppIcon icon="sparkles" size={28} />
            <AppIcon icon="workflow" size={28} />
            <AppIcon icon="layout-grid" size={28} />
            <AppIcon icon="database" size={28} />
          </div>
          <h2 className="text-[clamp(1.35rem,1.9vw,1.8rem)] leading-tight font-medium tracking-[-0.025em]">
            Describe it. Hivy builds it.
          </h2>
          <p className="mt-4 max-w-[37ch] text-[0.95rem] leading-6 text-muted">
            Tell Hivy what you need in plain English and it wires blocks,
            models, and integrations into a working agent.
          </p>
        </div>
      </div>
    </section>
  )
}

export function LandingCta() {
  return (
    <section
      id="contact"
      className="mx-auto flex min-h-[440px] w-[calc(100%-2rem)] max-w-[1300px] flex-col items-center justify-center text-center"
    >
      <h2 className="text-[clamp(2.3rem,4vw,3.5rem)] leading-none font-medium tracking-[-0.05em]">
        Build your first agent today.
      </h2>
      <div className="mt-7 flex items-center gap-2">
        <Link href="/auth/signup">
          <Button size="sm">Get started</Button>
        </Link>
        <Link href="mailto:sales@usehivy.com">
          <Button size="sm" variant="ghost">
            Contact sales
          </Button>
        </Link>
      </div>
    </section>
  )
}

export function LandingFooter() {
  return (
    <>
      <Separator />
      <footer
        id="resources"
        className="mx-auto w-[calc(100%-2rem)] max-w-[1300px] py-20"
      >
        <div className="grid gap-12 sm:grid-cols-3 lg:grid-cols-[0.8fr_repeat(7,1fr)]">
          <Link
            href="/home"
            className="h-fit text-lg font-semibold tracking-[-0.05em] text-foreground"
          >
            hivy
          </Link>
          {footerGroups.map((group) => (
            <div key={group.title}>
              <h2 className="text-sm font-medium">{group.title}</h2>
              <ul className="mt-4 space-y-2.5">
                {group.links.map((item) => (
                  <li key={item}>
                    <Link
                      href="#"
                      className="text-sm leading-5 text-muted hover:text-foreground"
                    >
                      {item}
                    </Link>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
        <p className="mt-28 text-xs text-muted">
          © 2026 Hivy. All rights reserved.
        </p>
      </footer>
    </>
  )
}
