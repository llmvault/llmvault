import type { Metadata } from "next"
import { Chip } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import {
  FeatureCopy,
  LandingCta,
  LandingFooter,
  LandingHero,
  ProductPlaceholder,
  TrustStrip,
  WorkflowPrompt,
  features,
  pillars,
} from "./_components/landing-shared"

export const metadata: Metadata = {
  title: "AI workspace for building agents",
  description:
    "Build, deploy, and manage AI agents visually, conversationally, or with code.",
}

export default function HomePage() {
  return (
    <main className="light min-h-screen bg-background text-foreground">
      <LandingHero />
      <TrustStrip />
      <WorkflowPrompt className="mt-28" />

      <section
        id="capabilities"
        className="mx-auto mt-28 w-[calc(100%-2rem)] max-w-[1300px]"
      >
        <h2 className="max-w-[680px] text-[clamp(1.8rem,2.7vw,2.65rem)] leading-[1.08] font-medium tracking-[-0.045em]">
          Everything your agents need, in one workspace.
          <br /> Build, run, and watch every agent.
        </h2>

        <div className="mt-20 grid gap-x-12 gap-y-14 sm:grid-cols-2 lg:grid-cols-4">
          {pillars.map((pillar) => (
            <div key={pillar.title}>
              <div className="flex h-32 items-center text-muted/45">
                <AppIcon icon={pillar.icon} size={92} strokeWidth={0.8} />
              </div>
              <h3 className="mt-6 text-xl font-medium tracking-[-0.02em]">
                {pillar.title}
              </h3>
              <p className="mt-3 max-w-[27ch] text-sm leading-5 text-muted">
                {pillar.description}
              </p>
            </div>
          ))}
        </div>
      </section>

      <section className="mx-auto mt-44 w-[calc(100%-2rem)] max-w-[1300px] space-y-32">
        {features.map((feature, index) => {
          const imageSide = index % 2 === 0 ? "left" : "right"

          return (
            <article
              key={feature.label}
              className="relative min-h-[520px] overflow-hidden rounded-sm border border-border bg-surface"
            >
              <Chip
                size="sm"
                className={`absolute top-3 z-10 ${imageSide === "left" ? "right-3" : "left-3"}`}
              >
                {feature.label}
              </Chip>
              <div className="grid min-h-[520px] md:grid-cols-[1fr_2fr]">
                {imageSide === "left" ? (
                  <>
                    <FeatureCopy
                      title={feature.title}
                      description={feature.description}
                      action={feature.action}
                      className="px-7 py-14 md:order-2 md:px-10 lg:px-12"
                    />
                    <div className="md:order-1">
                      <ProductPlaceholder label={feature.placeholder} />
                    </div>
                  </>
                ) : (
                  <>
                    <FeatureCopy
                      title={feature.title}
                      description={feature.description}
                      action={feature.action}
                      className="px-7 py-14 md:px-10 lg:px-12"
                    />
                    <ProductPlaceholder label={feature.placeholder} />
                  </>
                )}
              </div>
            </article>
          )
        })}
      </section>

      <LandingCta />
      <LandingFooter />
    </main>
  )
}
