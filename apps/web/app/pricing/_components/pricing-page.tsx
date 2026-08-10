import { Accordion, Button, Link } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { LandingHeader } from "../../home/_components/landing-header"
import { LandingFooter } from "../../home/_components/landing-shared"
import { PricingCalculator } from "./pricing-calculator"
import { PricingComparisonSection } from "./pricing-comparison"

const includedFeatures = [
  "Unlimited users",
  "Unlimited teams",
  "Unlimited agents",
  "Unlimited agent sessions",
  "Unlimited sandboxes",
  "Unlimited knowledge storage",
  "Unlimited knowledge sources",
  "Unlimited connections",
  "Access to every available model",
  "Unlimited agent drive storage",
  "Unlimited agent sheets",
  "Unlimited automations",
  "Webhook and connection triggers",
  "Role-based access control",
  "API and MCP access",
  "Model savings passed through",
] as const

function PricingHero() {
  return (
    <section className="mx-auto w-[calc(100%-2rem)] max-w-[1300px] pt-24">
      <div className="border-b border-border pb-12 md:pb-16">
        <h1 className="text-[clamp(3.3rem,7.5vw,7.5rem)] leading-[0.88] font-medium tracking-[-0.075em]">
          <span className="block">Pay for agent work.</span>
          <span className="block text-muted">Not another subscription.</span>
        </h1>
        <div className="mt-12 grid gap-8 border-t border-border pt-8 md:grid-cols-[1fr_300px] md:items-end">
          <p className="max-w-[720px] text-lg leading-8">
            Add $100 in credits and pay $112 once. Credits cover model usage and
            sandbox compute at one credit per active vCPU-minute.
          </p>
          <div className="md:text-right">
            <p className="text-7xl font-medium tracking-[-0.07em] md:text-8xl">
              12%
            </p>
            <p className="mt-2 text-xs text-muted">
              once, when you add credits
            </p>
          </div>
        </div>
      </div>
    </section>
  )
}

function CalculatorSection() {
  return (
    <section className="mx-auto mt-32 w-[calc(100%-2rem)] max-w-[1300px]">
      <p className="text-xs font-medium tracking-[0.08em] text-muted uppercase">
        Know the charge
      </p>
      <h2 className="mt-4 max-w-[720px] text-[clamp(2rem,3.5vw,3.2rem)] leading-[1] font-medium tracking-[-0.05em]">
        Pick a balance. See what you’ll pay.
      </h2>
      <p className="mt-5 max-w-[62ch] text-sm leading-6 text-muted">
        Move the slider to choose how much your agents can spend. The 12%
        deposit fee stays separate, so you know the total before adding credits.
      </p>
      <div className="mt-10">
        <PricingCalculator />
      </div>
    </section>
  )
}

function IncludedSection() {
  return (
    <section
      aria-label="Included features"
      className="mx-auto mt-28 w-[calc(100%-2rem)] max-w-[1300px]"
    >
      <div className="grid border-y border-border sm:grid-cols-2 lg:grid-cols-4">
        {includedFeatures.map((feature, index) => (
          <div
            key={feature}
            className={`flex min-h-16 items-center gap-3 border-border py-4 sm:px-5 ${index < includedFeatures.length - 1 ? "border-b" : ""} ${index >= includedFeatures.length - 2 ? "sm:border-b-0" : ""} ${index >= includedFeatures.length - 4 ? "lg:border-b-0" : ""} ${index % 2 === 1 ? "sm:border-l" : ""} ${index % 4 !== 0 ? "lg:border-l" : ""}`}
          >
            <span className="flex size-6 shrink-0 items-center justify-center rounded-full border border-border bg-surface-secondary text-foreground">
              <AppIcon icon="check" className="size-3.5" />
            </span>
            <span className="text-sm font-medium">{feature}</span>
          </div>
        ))}
      </div>
      <aside
        aria-label="Sandbox compute pricing"
        className="mt-6 flex max-w-[78ch] items-start gap-3"
      >
        <span className="mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-full bg-surface-secondary text-muted">
          <AppIcon icon="info" className="size-3.5" />
        </span>
        <div>
          <p className="text-sm font-medium">
            Every sandbox size is available.
          </p>
          <p className="mt-1 text-sm leading-6 text-muted">
            Sandbox compute costs one credit per active vCPU-minute. Nano and
            Small cost 1 credit per active minute, Medium costs 2, and Large
            costs 4. Idle time does not spend sandbox credits.
          </p>
        </div>
      </aside>
    </section>
  )
}

function AtCostSection() {
  const passThrough = [
    ["Model usage", "Charged at cost"],
    ["Sandbox compute", "1 credit / active vCPU-min"],
    ["Prompt-cache savings", "Kept in your balance"],
    ["Quantized-model savings", "Kept in your balance"],
  ] as const

  return (
    <section className="mx-auto mt-36 w-[calc(100%-2rem)] max-w-[1300px]">
      <div className="grid gap-16 rounded-sm bg-surface-secondary px-6 py-12 md:px-10 md:py-16 lg:grid-cols-[0.95fr_1.05fr] lg:px-16 lg:py-20">
        <div>
          <p className="text-xs font-medium tracking-[0.08em] text-muted uppercase">
            Your savings stay yours
          </p>
          <h2 className="mt-4 max-w-[620px] text-[clamp(2.2rem,4vw,3.8rem)] leading-[0.96] font-medium tracking-[-0.055em]">
            Cheaper models should lower your bill.
          </h2>
          <p className="mt-6 max-w-[58ch] text-sm leading-6 text-muted">
            Your balance covers model usage and active sandbox compute. When
            caching or a quantized model cuts model cost, you keep the
            difference.
          </p>
        </div>

        <div className="flex flex-col justify-between gap-10 lg:pt-5">
          <p className="max-w-[520px] text-[clamp(1.75rem,3vw,2.75rem)] leading-[1.04] font-medium tracking-[-0.045em]">
            Hivy adds <span className="text-primary">0%</span> to agent costs.
          </p>

          <div className="space-y-7">
            {passThrough.map(([source, result]) => (
              <div
                key={source}
                className="grid grid-cols-[minmax(0,1fr)_auto_minmax(0,0.8fr)] items-center gap-4 text-sm"
              >
                <span className="text-muted">{source}</span>
                <AppIcon
                  icon="arrow-right"
                  aria-hidden="true"
                  className="size-4 text-muted"
                />
                <span className="font-medium">{result}</span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </section>
  )
}

function PricingFaq() {
  const questions = [
    {
      question: "Will Hivy charge me every month?",
      answer:
        "No. There’s no monthly subscription and no per-seat plan. Add credits when your agents need a working balance.",
    },
    {
      question: "What do I get without a plan?",
      answer:
        "Unlimited users, teams, agents, sessions, sandboxes, knowledge sources, and connections. Choose any sandbox size and pay only for its active vCPU time.",
    },
    {
      question: "How is sandbox compute charged?",
      answer:
        "One credit buys one active vCPU-minute. Nano and Small use 1 vCPU, Medium uses 2, and Large uses 4. Billing accumulates by the second across active agent turns; idle time does not spend sandbox credits.",
    },
    {
      question: "Can I choose any sandbox size?",
      answer:
        "Yes. There are no deposit tiers or sandbox-size unlocks. Larger sandboxes simply spend credits faster while they are active.",
    },
    {
      question: "What spends my credits?",
      answer:
        "Model usage and active sandbox compute. Sandbox compute costs one credit per active vCPU-minute; idle time costs nothing.",
    },
    {
      question: "Do you mark up model prices?",
      answer:
        "No. Your balance pays the model or provider price. If that price drops, your agent costs drop with it.",
    },
    {
      question: "When do I pay the 12% fee?",
      answer:
        "Only when you add credits. A $100 balance carries a $12 fee, so you pay $112 once and receive the full $100 to spend.",
    },
    {
      question: "Can I use quantized models?",
      answer:
        "Yes, when an available provider offers one. Quantized models use fewer resources, and the lower cost stays in your balance.",
    },
  ]

  return (
    <section className="mx-auto mt-36 w-[calc(100%-2rem)] max-w-[920px]">
      <p className="text-xs font-medium tracking-[0.08em] text-muted uppercase">
        Pricing questions
      </p>
      <h2 className="mt-4 text-[clamp(2rem,3.5vw,3rem)] leading-none font-medium tracking-[-0.05em]">
        Before you add credits.
      </h2>
      <Accordion className="mt-10 border-y border-border">
        {questions.map((item) => (
          <Accordion.Item key={item.question} id={item.question}>
            <Accordion.Heading>
              <Accordion.Trigger className="focus-visible:ring-ring w-full px-0 py-5 text-base font-medium outline-none focus-visible:ring-2">
                {item.question}
                <Accordion.Indicator className="pricing-faq-indicator ml-auto size-[17px] shrink-0 text-foreground transition-transform! duration-250! ease-[cubic-bezier(0.22,1,0.36,1)]! data-[expanded=true]:rotate-45! motion-reduce:transition-none!">
                  <AppIcon icon="plus" size={17} />
                </Accordion.Indicator>
              </Accordion.Trigger>
            </Accordion.Heading>
            <Accordion.Panel className="pricing-faq-panel transition-[height,opacity]! duration-300! ease-[cubic-bezier(0.22,1,0.36,1)]! [interpolate-size:allow-keywords] motion-reduce:transition-none!">
              <Accordion.Body className="max-w-[68ch] px-0 pt-0 pb-5 text-sm leading-6 text-muted">
                {item.answer}
              </Accordion.Body>
            </Accordion.Panel>
          </Accordion.Item>
        ))}
      </Accordion>
    </section>
  )
}

function PricingCta() {
  return (
    <section className="mx-auto flex min-h-[440px] w-[calc(100%-2rem)] max-w-[1300px] flex-col items-center justify-center text-center">
      <h2 className="max-w-[800px] text-[clamp(2.5rem,4.5vw,4.3rem)] leading-[0.94] font-medium tracking-[-0.06em]">
        Start free. Add $5 when you’re ready.
      </h2>
      <p className="mt-5 max-w-[54ch] text-sm leading-6 text-muted">
        Set up your workspace without a subscription. Your first deposit can be
        as little as $5.
      </p>
      <div className="mt-7">
        <Link href="/auth/signup">
          <Button size="sm">Start for free</Button>
        </Link>
      </div>
    </section>
  )
}

export function PricingPage() {
  return (
    <main className="marketing-link-scope min-h-screen bg-background text-foreground">
      <LandingHeader />
      <PricingHero />
      <CalculatorSection />
      <PricingComparisonSection />
      <IncludedSection />
      <AtCostSection />
      <PricingFaq />
      <PricingCta />
      <LandingFooter />
    </main>
  )
}
