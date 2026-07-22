import type { Metadata } from "next"
import Image from "next/image"
import { Chip } from "@heroui/react"
import { HomeFeatureStories } from "./_components/home-feature-stories"
import { ModelChoiceSection } from "./_components/model-choice-section"
import {
  FeatureCopy,
  LandingCta,
  LandingFooter,
  LandingHero,
  ProductPlaceholder,
  features,
} from "./_components/landing-shared"

export const metadata: Metadata = {
  title: "AI workspace for building agents",
  description:
    "Build, deploy, and manage AI agents visually, conversationally, or with code.",
}

const homeFeatures = features.filter(
  (feature) => feature.title !== "Watch every run, end to end."
)

const featureScreenshots = {
  Integrate: {
    light: "/images/marketing/connections-light-mode.png",
    dark: "/images/marketing/connections-dark-mode.png",
    alt: "Hivy connection catalog with databases and company integrations",
  },
  Context: {
    light: "/images/marketing/knowledge-base-light-mode.png",
    dark: "/images/marketing/knowledge-base-dark-mode.png",
    alt: "Hivy knowledge source setup with GitHub, Notion, Slack, Linear, and website sources",
  },
} as const

function FeatureScreenshot({
  screenshot,
  side,
}: {
  screenshot: (typeof featureScreenshots)[keyof typeof featureScreenshots]
  side: "left" | "right"
}) {
  const frameClassName =
    side === "left"
      ? "absolute top-16 right-16 bottom-0 left-0 overflow-hidden rounded-tr-sm border-t border-r border-border bg-background"
      : "absolute top-16 right-0 bottom-0 left-16 overflow-hidden rounded-tl-sm border-t border-l border-border bg-background"
  const imageClassName =
    side === "left"
      ? "object-contain object-left-bottom"
      : "object-contain object-right-bottom"

  return (
    <div className="relative h-full min-h-80 overflow-hidden bg-surface-secondary md:min-h-0">
      <div className={frameClassName}>
        <Image
          src={screenshot.light}
          alt={screenshot.alt}
          fill
          sizes="(min-width: 1300px) 850px, (min-width: 768px) 66vw, 100vw"
          className={`${imageClassName} dark:hidden`}
        />
        <Image
          src={screenshot.dark}
          alt=""
          fill
          sizes="(min-width: 1300px) 850px, (min-width: 768px) 66vw, 100vw"
          className={`hidden ${imageClassName} dark:block`}
        />
      </div>
    </div>
  )
}

export default function HomePage() {
  return (
    <main className="marketing-link-scope min-h-screen bg-background text-foreground">
      <LandingHero />
      <ModelChoiceSection />

      <HomeFeatureStories />

      <section className="mx-auto mt-44 w-[calc(100%-2rem)] max-w-[1300px] space-y-32">
        {homeFeatures.map((feature, index) => {
          const imageSide = index % 2 === 0 ? "left" : "right"
          const screenshot =
            feature.label === "Integrate" || feature.label === "Context"
              ? featureScreenshots[feature.label]
              : undefined

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
              <div
                className={`grid min-h-[520px] ${
                  imageSide === "left"
                    ? "md:grid-cols-[2fr_1fr]"
                    : "md:grid-cols-[1fr_2fr]"
                }`}
              >
                {imageSide === "left" ? (
                  <>
                    <FeatureCopy
                      title={feature.title}
                      description={feature.description}
                      action={feature.action}
                      className="px-7 py-14 md:order-2 md:px-10 lg:px-12"
                    />
                    <div className="h-full md:order-1">
                      {screenshot ? (
                        <FeatureScreenshot
                          screenshot={screenshot}
                          side="left"
                        />
                      ) : (
                        <ProductPlaceholder label={feature.placeholder} />
                      )}
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
                    {screenshot ? (
                      <FeatureScreenshot screenshot={screenshot} side="right" />
                    ) : (
                      <ProductPlaceholder label={feature.placeholder} />
                    )}
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
