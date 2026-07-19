import { Chip, Link } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import {
  LandingCta,
  LandingFooter,
  LandingHero,
  ProductPlaceholder,
  TrustStrip,
  features,
  pillars,
} from "./landing-shared"

export type VariantMode =
  | "canvas"
  | "chapters"
  | "timeline"
  | "ledger"
  | "bands"
  | "night"

const modeCopy: Record<
  VariantMode,
  { eyebrow: string; heading: string; description: string }
> = {
  canvas: {
    eyebrow: "One workspace",
    heading: "Every surface your agents need, arranged as one working canvas.",
    description:
      "Move from connected tools to grounded context, visual logic, and live run data without leaving the workspace.",
  },
  chapters: {
    eyebrow: "The Hivy loop",
    heading: "Four focused chapters. One continuous operating loop.",
    description:
      "Each stage has room to explain itself, while the workflow stays legible from first connection to finished run.",
  },
  timeline: {
    eyebrow: "Agent lifecycle",
    heading: "Follow the work from first input to final result.",
    description:
      "A connected sequence shows how tools, knowledge, logic, and observability become one dependable agent system.",
  },
  ledger: {
    eyebrow: "System record",
    heading: "A precise account of how useful agents are made.",
    description:
      "The product is organized like an operating ledger: numbered stages, clear inputs, and visible outcomes.",
  },
  bands: {
    eyebrow: "Workspace layers",
    heading: "A full stack for agent work, expressed in layers.",
    description:
      "Each layer carries one responsibility, then hands cleanly into the next without hiding the underlying work.",
  },
  night: {
    eyebrow: "Control room",
    heading: "See the entire agent system, even when the work gets complex.",
    description:
      "A darker operational view gives every stage a clear boundary while keeping the complete workflow connected.",
  },
}

function WorkflowCard() {
  return (
    <div className="w-full max-w-[330px] rounded-lg border border-border bg-surface shadow-sm">
      <div className="flex items-center gap-3 border-b border-border px-4 py-3">
        <AppIcon icon="github" size={23} />
        <span className="font-medium">GitHub</span>
      </div>
      <div className="flex items-center justify-between px-4 py-3 text-sm text-muted">
        <span>Trigger</span>
        <span className="text-foreground">PR opened</span>
      </div>
    </div>
  )
}

export function VariantWorkflow({ mode }: { mode: VariantMode }) {
  if (mode === "ledger") {
    return (
      <section className="mx-auto mt-28 w-[calc(100%-2rem)] max-w-[1300px] border-y border-border py-16">
        <div className="grid gap-12 lg:grid-cols-[130px_1fr_1fr] lg:items-center">
          <span className="text-[5rem] leading-none font-medium tracking-[-0.08em] text-muted/30">
            01
          </span>
          <div>
            <p className="text-xs tracking-[0.12em] text-muted uppercase">
              Start in plain English
            </p>
            <h2 className="mt-5 text-3xl font-medium tracking-[-0.04em]">
              Describe it. Hivy builds it.
            </h2>
            <p className="mt-4 max-w-[42ch] text-sm leading-6 text-muted">
              Tell Hivy what you need in plain English and it wires blocks,
              models, and integrations into a working agent.
            </p>
          </div>
          <div className="flex justify-center bg-surface-secondary p-12">
            <WorkflowCard />
          </div>
        </div>
      </section>
    )
  }

  if (mode === "bands") {
    return (
      <section className="mt-28 bg-surface-secondary py-24">
        <div className="mx-auto grid w-[calc(100%-2rem)] max-w-[1300px] gap-14 md:grid-cols-[0.9fr_1.1fr] md:items-center">
          <div>
            <Chip size="sm">Describe</Chip>
            <h2 className="mt-8 text-[clamp(2rem,4vw,4rem)] leading-none font-medium tracking-[-0.05em]">
              Describe it. Hivy builds it.
            </h2>
            <p className="mt-6 max-w-[46ch] text-base leading-7 text-muted">
              Tell Hivy what you need in plain English and it wires blocks,
              models, and integrations into a working agent.
            </p>
          </div>
          <div className="flex min-h-[360px] items-center justify-center rounded-sm bg-surface-tertiary p-8">
            <WorkflowCard />
          </div>
        </div>
      </section>
    )
  }

  if (mode === "night") {
    return (
      <section className="dark mt-28 bg-background py-28 text-foreground">
        <div className="mx-auto grid w-[calc(100%-2rem)] max-w-[1300px] gap-14 lg:grid-cols-[1.2fr_0.8fr] lg:items-center">
          <div className="relative flex min-h-[480px] items-center justify-center overflow-hidden rounded-sm border border-border bg-surface-secondary p-8">
            <div className="absolute inset-x-10 top-1/2 h-px bg-border" />
            <WorkflowCard />
          </div>
          <div>
            <p className="text-xs tracking-[0.12em] text-muted uppercase">
              Natural-language control
            </p>
            <h2 className="mt-6 text-[clamp(2rem,4vw,4rem)] leading-none font-medium tracking-[-0.05em]">
              Describe it. Hivy builds it.
            </h2>
            <p className="mt-6 max-w-[42ch] text-base leading-7 text-muted">
              Tell Hivy what you need in plain English and it wires blocks,
              models, and integrations into a working agent.
            </p>
          </div>
        </div>
      </section>
    )
  }

  if (mode === "timeline") {
    return (
      <section className="mx-auto mt-28 w-[calc(100%-2rem)] max-w-[1300px] py-16">
        <div className="grid gap-14 lg:grid-cols-[0.8fr_1.2fr] lg:items-center">
          <div>
            <Chip size="sm">Prompt to production</Chip>
            <h2 className="mt-7 text-[clamp(2rem,4vw,4rem)] leading-none font-medium tracking-[-0.05em]">
              Describe it. Hivy builds it.
            </h2>
            <p className="mt-6 max-w-[42ch] text-base leading-7 text-muted">
              Tell Hivy what you need in plain English and it wires blocks,
              models, and integrations into a working agent.
            </p>
          </div>
          <div className="relative flex items-center justify-between gap-4 py-16">
            <div className="absolute right-8 left-8 h-px bg-border" />
            {[
              ["github", "Trigger"],
              ["sparkles", "Describe"],
              ["workflow", "Compose"],
              ["monitor", "Run"],
            ].map(([icon, label]) => (
              <div
                key={label}
                className="relative flex flex-col items-center gap-3"
              >
                <span className="flex size-14 items-center justify-center rounded-full border border-border bg-surface">
                  <AppIcon icon={icon} size={21} />
                </span>
                <span className="text-xs font-medium">{label}</span>
              </div>
            ))}
          </div>
        </div>
      </section>
    )
  }

  const isChapters = mode === "chapters"

  return (
    <section className="mx-auto mt-28 w-[calc(100%-2rem)] max-w-[1300px] overflow-hidden rounded-sm border border-border bg-surface">
      <div
        className={`grid min-h-[600px] ${isChapters ? "lg:grid-cols-[0.72fr_1.28fr]" : "lg:grid-cols-[1.25fr_0.75fr]"}`}
      >
        <div
          className={`flex items-center justify-center bg-surface-secondary p-10 ${isChapters ? "lg:order-2" : ""}`}
        >
          <WorkflowCard />
        </div>
        <div className="flex flex-col justify-center p-10 lg:p-14">
          <Chip size="sm" className="w-fit">
            Describe
          </Chip>
          <h2 className="mt-8 text-3xl font-medium tracking-[-0.04em]">
            Describe it. Hivy builds it.
          </h2>
          <p className="mt-5 max-w-[40ch] text-base leading-7 text-muted">
            Tell Hivy what you need in plain English and it wires blocks,
            models, and integrations into a working agent.
          </p>
        </div>
      </div>
    </section>
  )
}

export function VariantOverview({ mode }: { mode: VariantMode }) {
  const copy = modeCopy[mode]
  const isNight = mode === "night"
  const isLedger = mode === "ledger"
  const isTimeline = mode === "timeline"
  const isBands = mode === "bands"

  return (
    <section
      id="capabilities"
      className={`${isNight ? "dark bg-background py-28 text-foreground" : isBands ? "bg-surface-tertiary py-28" : "mx-auto mt-32 w-[calc(100%-2rem)] max-w-[1300px]"}`}
    >
      <div
        className={
          isNight || isBands ? "mx-auto w-[calc(100%-2rem)] max-w-[1300px]" : ""
        }
      >
        <div className="grid gap-10 lg:grid-cols-[1.1fr_0.9fr] lg:items-end">
          <div>
            <p className="text-xs font-medium tracking-[0.12em] text-muted uppercase">
              {copy.eyebrow}
            </p>
            <h2 className="mt-6 max-w-[760px] text-[clamp(2.3rem,4.5vw,4.5rem)] leading-[0.96] font-medium tracking-[-0.055em]">
              {copy.heading}
            </h2>
          </div>
          <div className="lg:pb-2">
            <p className="max-w-[46ch] text-base leading-7 text-muted">
              {copy.description}
            </p>
            <p className="mt-5 max-w-[46ch] text-sm leading-6 text-muted">
              Everything your agents need, in one workspace. Build, run, and
              watch every agent.
            </p>
          </div>
        </div>

        <div
          className={`mt-20 ${isLedger ? "divide-y divide-border border-y border-border" : isTimeline ? "relative grid gap-8 before:absolute before:top-8 before:right-[12%] before:left-[12%] before:h-px before:bg-border md:grid-cols-4" : "grid gap-4 sm:grid-cols-2 lg:grid-cols-4"}`}
        >
          {pillars.map((pillar, index) => (
            <div
              key={pillar.title}
              className={
                isLedger
                  ? "grid gap-5 py-6 sm:grid-cols-[70px_160px_1fr] sm:items-center"
                  : isTimeline
                    ? "relative pt-1 text-center"
                    : `${isNight ? "border-border bg-surface" : "border-border bg-surface"} rounded-sm border p-7`
              }
            >
              {isLedger ? (
                <span className="text-sm text-muted">0{index + 2}</span>
              ) : (
                <span
                  className={`flex size-16 items-center justify-center ${isTimeline ? "relative mx-auto rounded-full border border-border bg-surface" : ""}`}
                >
                  <AppIcon icon={pillar.icon} size={isTimeline ? 22 : 34} />
                </span>
              )}
              <h3
                className={`${isTimeline ? "mt-5" : "mt-6"} text-lg font-medium`}
              >
                {pillar.title}
              </h3>
              <p
                className={`${isTimeline ? "mt-3" : "mt-3"} text-sm leading-5 text-muted`}
              >
                {pillar.description}
              </p>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}

function FeatureAction({ action }: { action?: string }) {
  return action ? (
    <Link
      href="#"
      className="mt-7 inline-flex items-center gap-2 text-sm font-medium text-foreground"
    >
      {action}
      <AppIcon icon="arrow-right" size={15} />
    </Link>
  ) : null
}

export function VariantFeatureSections({ mode }: { mode: VariantMode }) {
  return (
    <>
      {features.map((feature, index) => {
        const reverse = index % 2 === 1
        const number = `0${index + 1}`

        if (mode === "ledger") {
          return (
            <section
              key={feature.label}
              className="mx-auto mt-24 w-[calc(100%-2rem)] max-w-[1300px] border-t border-border pt-10"
            >
              <div className="grid gap-10 lg:grid-cols-[100px_0.8fr_1.2fr]">
                <span className="text-4xl font-medium tracking-[-0.05em] text-muted/40">
                  {number}
                </span>
                <div className="pt-2">
                  <Chip size="sm">{feature.label}</Chip>
                  <h2 className="mt-7 text-3xl leading-tight font-medium tracking-[-0.04em]">
                    {feature.title}
                  </h2>
                  <p className="mt-5 text-sm leading-6 text-muted">
                    {feature.description}
                  </p>
                  <FeatureAction action={feature.action} />
                </div>
                <div className="aspect-[1.35/1] overflow-hidden bg-surface">
                  <ProductPlaceholder label={feature.placeholder} />
                </div>
              </div>
            </section>
          )
        }

        if (mode === "bands") {
          return (
            <section
              key={feature.label}
              className={`${index % 2 === 0 ? "bg-surface" : "bg-surface-secondary"} py-28`}
            >
              <div className="mx-auto grid min-h-[520px] w-[calc(100%-2rem)] max-w-[1300px] gap-14 md:grid-cols-2 md:items-center">
                <div className={reverse ? "md:order-2" : ""}>
                  <div className="flex items-center gap-4">
                    <Chip size="sm">{feature.label}</Chip>
                    <span className="text-xs text-muted">{number} / 04</span>
                  </div>
                  <h2 className="mt-9 max-w-[560px] text-[clamp(2rem,4vw,4rem)] leading-[1.02] font-medium tracking-[-0.05em]">
                    {feature.title}
                  </h2>
                  <p className="mt-6 max-w-[48ch] text-base leading-7 text-muted">
                    {feature.description}
                  </p>
                  <FeatureAction action={feature.action} />
                </div>
                <div
                  className={`aspect-[1.2/1] overflow-hidden rounded-sm border border-border ${reverse ? "md:order-1" : ""}`}
                >
                  <ProductPlaceholder label={feature.placeholder} />
                </div>
              </div>
            </section>
          )
        }

        if (mode === "night") {
          return (
            <section
              key={feature.label}
              className="dark border-t border-border bg-background py-28 text-foreground"
            >
              <div className="mx-auto grid min-h-[520px] w-[calc(100%-2rem)] max-w-[1300px] gap-14 md:grid-cols-[0.8fr_1.2fr] md:items-center">
                <div className={reverse ? "md:order-2" : ""}>
                  <div className="flex items-center gap-4">
                    <span className="flex size-12 items-center justify-center rounded-full border border-border bg-surface">
                      <AppIcon icon={feature.icon} size={20} />
                    </span>
                    <Chip size="sm">{feature.label}</Chip>
                  </div>
                  <h2 className="mt-9 text-[clamp(2rem,4vw,4rem)] leading-[1.02] font-medium tracking-[-0.05em]">
                    {feature.title}
                  </h2>
                  <p className="mt-6 max-w-[44ch] text-base leading-7 text-muted">
                    {feature.description}
                  </p>
                  <FeatureAction action={feature.action} />
                </div>
                <div
                  className={`aspect-[1.35/1] overflow-hidden rounded-sm border border-border bg-surface ${reverse ? "md:order-1" : ""}`}
                >
                  <ProductPlaceholder label={feature.placeholder} />
                </div>
              </div>
            </section>
          )
        }

        if (mode === "timeline") {
          return (
            <section
              key={feature.label}
              className="relative mx-auto mt-28 w-[calc(100%-2rem)] max-w-[1300px]"
            >
              <div className="absolute top-0 bottom-0 left-6 w-px bg-border md:left-1/2" />
              <div className="relative grid gap-10 pl-20 md:grid-cols-2 md:gap-20 md:pl-0">
                <span className="absolute top-0 left-0 flex size-12 items-center justify-center rounded-full border border-border bg-surface md:left-1/2 md:-translate-x-1/2">
                  <AppIcon icon={feature.icon} size={20} />
                </span>
                <div className={reverse ? "md:order-2 md:pl-10" : "md:pr-10"}>
                  <Chip size="sm">{feature.label}</Chip>
                  <h2 className="mt-7 text-[clamp(2rem,3.5vw,3.5rem)] leading-[1.02] font-medium tracking-[-0.05em]">
                    {feature.title}
                  </h2>
                  <p className="mt-5 text-base leading-7 text-muted">
                    {feature.description}
                  </p>
                  <FeatureAction action={feature.action} />
                </div>
                <div
                  className={`aspect-[1.15/1] overflow-hidden rounded-sm border border-border ${reverse ? "md:order-1" : "md:order-2"}`}
                >
                  <ProductPlaceholder label={feature.placeholder} />
                </div>
              </div>
            </section>
          )
        }

        const isChapters = mode === "chapters"

        return (
          <section
            key={feature.label}
            className={`mx-auto w-[calc(100%-2rem)] max-w-[1300px] ${isChapters ? "mt-40 min-h-[780px]" : "mt-24"}`}
          >
            <div
              className={`grid gap-10 ${isChapters ? "lg:grid-cols-[320px_1fr]" : "overflow-hidden rounded-sm border border-border bg-surface md:min-h-[560px] md:grid-cols-[0.82fr_1.18fr]"}`}
            >
              <div
                className={`${isChapters ? "h-fit lg:sticky lg:top-12" : "flex flex-col justify-center p-9 lg:p-12"} ${reverse && !isChapters ? "md:order-2" : ""}`}
              >
                <div className="flex items-center gap-4">
                  <Chip size="sm">{feature.label}</Chip>
                  <span className="text-xs text-muted">{number}</span>
                </div>
                <h2 className="mt-8 text-[clamp(2rem,3.5vw,3.5rem)] leading-[1.02] font-medium tracking-[-0.05em]">
                  {feature.title}
                </h2>
                <p className="mt-6 text-base leading-7 text-muted">
                  {feature.description}
                </p>
                <FeatureAction action={feature.action} />
              </div>
              <div
                className={`${isChapters ? "aspect-[1.05/1] overflow-hidden rounded-sm border border-border" : ""} ${reverse && !isChapters ? "md:order-1" : ""}`}
              >
                <ProductPlaceholder label={feature.placeholder} />
              </div>
            </div>
          </section>
        )
      })}
    </>
  )
}

export function CompleteVariantPage({
  mode,
  nextHref,
  nextLabel,
}: {
  mode: VariantMode
  nextHref: string
  nextLabel: string
}) {
  return (
    <main className="light min-h-screen bg-background text-foreground">
      <LandingHero />
      <TrustStrip />
      <VariantWorkflow mode={mode} />
      <VariantOverview mode={mode} />
      <VariantFeatureSections mode={mode} />
      <div className="mx-auto mt-28 flex w-[calc(100%-2rem)] max-w-[1300px] justify-end">
        <Link
          href={nextHref}
          className="inline-flex items-center gap-2 text-sm font-medium text-foreground"
        >
          {nextLabel} <AppIcon icon="arrow-right" size={15} />
        </Link>
      </div>
      <LandingCta />
      <LandingFooter />
    </main>
  )
}
