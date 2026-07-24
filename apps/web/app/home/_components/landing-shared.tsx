import { Button, Link, Separator, Skeleton } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { footerGroups } from "./landing-footer-links"
import { LandingHeader } from "./landing-header"
import { MarketingLogo } from "./marketing-logo"

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

type PlatformHighlight = {
  icon: string
  title: string
  description: string
}

export function PlatformHighlights({
  items,
  className = "",
}: {
  items: readonly PlatformHighlight[]
  className?: string
}) {
  return (
    <div
      className={`grid border-x border-b border-border md:grid-cols-3 ${className}`}
    >
      {items.map((item, index) => (
        <article
          key={item.title}
          className={`min-h-44 p-6 md:p-7 ${
            index < items.length - 1
              ? "border-b border-border md:border-r md:border-b-0"
              : ""
          }`}
        >
          <span className="flex size-9 items-center justify-center rounded-sm bg-surface-secondary text-muted">
            <AppIcon icon={item.icon} size={17} />
          </span>
          <h3 className="mt-6 text-base font-medium tracking-[-0.02em]">
            {item.title}
          </h3>
          <p className="mt-2 max-w-[36ch] text-sm leading-6 text-muted">
            {item.description}
          </p>
        </article>
      ))}
    </div>
  )
}

type LandingHeroProps = {
  titleLines?: readonly [string, string]
  description?: string
  primaryAction?: {
    label: string
    href: string
  }
  secondaryAction?: {
    label: string
    href: string
  }
  placeholderLabel?: string
}

function LandingHeroActions({
  primaryAction = { label: "Watch a 2min demo", href: "#contact" },
  secondaryAction = { label: "Start for free", href: "/auth/signup" },
  className = "mt-8",
}: Pick<LandingHeroProps, "primaryAction" | "secondaryAction"> & {
  className?: string
}) {
  return (
    <div className={`flex items-center gap-2 ${className}`}>
      <Link href={primaryAction.href}>
        <Button size="sm">{primaryAction.label}</Button>
      </Link>
      <Link href={secondaryAction.href}>
        <Button size="sm" variant="ghost">
          {secondaryAction.label}
        </Button>
      </Link>
    </div>
  )
}

export function LandingHero({
  titleLines = [
    "Productive ai agents for your entire team.",
    "With no monthly subscriptions",
  ],
  description = "Open source, with 1,000+ integrations and every major LLM. Build, deploy, and manage agents visually, conversationally, or with code.",
  primaryAction = { label: "Watch a 2min demo", href: "#contact" },
  secondaryAction = { label: "Start for free", href: "/auth/signup" },
  placeholderLabel = "Main Hivy workspace product screenshot",
}: LandingHeroProps = {}) {
  return (
    <>
      <LandingHeader />
      <section className="mx-auto w-[calc(100%-2rem)] max-w-[1300px] pt-28">
        <div>
          <h1 className="max-w-[980px] text-[clamp(2.35rem,4vw,3rem)] leading-[1.02] font-medium tracking-[-0.045em] text-balance">
            <span className="block md:whitespace-nowrap">{titleLines[0]}</span>
            <span className="block">{titleLines[1]}</span>
          </h1>
          <p className="mt-7 max-w-[630px] text-[1rem] leading-6 text-muted">
            {description}
          </p>
          <LandingHeroActions
            primaryAction={primaryAction}
            secondaryAction={secondaryAction}
          />
        </div>

        <div
          id="product"
          className="mt-16 aspect-[1.82/1] overflow-hidden rounded-sm bg-surface-tertiary p-[7%]"
        >
          <ProductPlaceholder label={placeholderLabel} />
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

export function LandingCta() {
  return (
    <section
      id="contact"
      className="mx-auto flex min-h-[440px] w-[calc(100%-2rem)] max-w-[1300px] flex-col items-center justify-center text-center"
    >
      <h2 className="text-[clamp(2.3rem,4vw,3.5rem)] leading-none font-medium tracking-[-0.05em]">
        Build your first agent today.
      </h2>
      <div className="mt-7">
        <Link href="/auth/signup">
          <Button size="sm">Get started</Button>
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
        <div className="grid gap-12 sm:grid-cols-2 lg:grid-cols-[0.8fr_repeat(3,1fr)]">
          <Link
            href="/"
            aria-label="Hivy home"
            className="inline-flex h-10 w-fit items-center text-foreground"
          >
            <MarketingLogo className="h-10 w-auto" />
          </Link>
          {footerGroups.map((group) => (
            <div key={group.title}>
              <h2 className="text-sm font-medium">{group.title}</h2>
              <ul className="mt-4 space-y-2.5">
                {group.links.map((item) => (
                  <li key={item.href}>
                    <Link
                      href={item.href}
                      className="text-sm leading-5 text-muted hover:text-foreground"
                    >
                      {item.label}
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
