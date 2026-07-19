import { Button, Link } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { LandingHeader } from "../../home/_components/landing-header"
import { LandingFooter } from "../../home/_components/landing-shared"
import { PricingCalculator, type CalculatorMode } from "./pricing-calculator"

type PricingVariant = CalculatorMode

const variantLinks = [
  { href: "/pricing", label: "One fee", value: "plain" },
  {
    href: "/pricing/variant-2",
    label: "Unlimited",
    value: "unlimited",
  },
  { href: "/pricing/variant-3", label: "Receipt", value: "receipt" },
  {
    href: "/pricing/variant-4",
    label: "Manifesto",
    value: "manifesto",
  },
] as const

const unlimitedItems = [
  ["Agents", "Build as many specialists as your team needs."],
  ["Sessions", "Run agents as often as the work demands."],
  ["Sandboxes", "Give every run a secure place to work."],
  ["Knowledge base storage", "Keep the context your agents need."],
] as const

function VariantNav({ active }: { active: PricingVariant }) {
  return (
    <nav
      aria-label="Pricing explorations"
      className="flex flex-wrap items-center gap-1"
    >
      {variantLinks.map((item) => (
        <Link key={item.href} href={item.href}>
          <Button
            size="sm"
            variant={active === item.value ? "secondary" : "ghost"}
          >
            {item.label}
          </Button>
        </Link>
      ))}
    </nav>
  )
}

function SampleReceipt() {
  return (
    <div className="rounded-sm border border-border bg-surface p-6 md:p-8">
      <div className="flex items-center justify-between gap-4 border-b border-border pb-5">
        <span className="text-xs font-medium tracking-[0.08em] text-muted uppercase">
          Credit deposit
        </span>
        <span className="text-xs text-muted">Example</span>
      </div>
      <div className="space-y-4 py-6 text-sm">
        <div className="flex justify-between gap-6">
          <span className="text-muted">Credit value</span>
          <span>$100.00</span>
        </div>
        <div className="flex justify-between gap-6">
          <span className="text-muted">Deposit fee (12%)</span>
          <span>$12.00</span>
        </div>
      </div>
      <div className="flex items-end justify-between gap-6 border-t border-border pt-5">
        <span className="text-sm font-medium">Pay once</span>
        <span className="text-3xl font-medium tracking-[-0.05em]">$112.00</span>
      </div>
      <p className="mt-6 border-t border-border pt-5 text-xs leading-5 text-muted">
        Then use the $100 balance at model cost with 0% Hivy markup.
      </p>
    </div>
  )
}

function PricingHero({ variant }: { variant: PricingVariant }) {
  if (variant === "unlimited") {
    return (
      <section className="mx-auto w-[calc(100%-2rem)] max-w-[1300px] pt-20 text-center">
        <div className="flex justify-start">
          <VariantNav active={variant} />
        </div>
        <p className="mt-20 text-xs font-medium tracking-[0.1em] text-muted uppercase">
          One transparent fee
        </p>
        <h1 className="mx-auto mt-5 max-w-[900px] text-[clamp(2.7rem,5vw,4.8rem)] leading-[0.96] font-medium tracking-[-0.06em] text-balance">
          Unlimited means unlimited.
        </h1>
        <p className="mx-auto mt-7 max-w-[660px] text-base leading-7 text-muted">
          Unlimited agents, sessions, sandboxes, and knowledge base storage. No
          monthly subscription. Add credits and pay a 12% fee once.
        </p>
        <div className="mt-14 grid overflow-hidden rounded-sm border border-border bg-surface text-left sm:grid-cols-2 lg:grid-cols-4">
          {unlimitedItems.map(([title]) => (
            <div
              key={title}
              className="border-border p-6 sm:border-l sm:first:border-l-0"
            >
              <p className="text-5xl font-medium tracking-[-0.06em]">∞</p>
              <p className="mt-5 text-sm font-medium">{title}</p>
            </div>
          ))}
        </div>
      </section>
    )
  }

  if (variant === "receipt") {
    return (
      <section className="mx-auto w-[calc(100%-2rem)] max-w-[1300px] pt-20">
        <VariantNav active={variant} />
        <div className="mt-16 grid items-center gap-12 lg:grid-cols-[1.05fr_0.72fr] lg:gap-20">
          <div>
            <p className="text-xs font-medium tracking-[0.1em] text-muted uppercase">
              Pricing you can audit at a glance
            </p>
            <h1 className="mt-5 max-w-[780px] text-[clamp(2.7rem,5vw,4.7rem)] leading-[0.96] font-medium tracking-[-0.06em] text-balance">
              The whole price fits on one receipt.
            </h1>
            <p className="mt-7 max-w-[620px] text-base leading-7 text-muted">
              Add credits. Pay 12% at deposit. Run unlimited agents, sessions,
              and sandboxes with unlimited knowledge base storage.
            </p>
          </div>
          <SampleReceipt />
        </div>
      </section>
    )
  }

  if (variant === "manifesto") {
    return (
      <section className="mx-auto w-[calc(100%-2rem)] max-w-[1300px] pt-20">
        <VariantNav active={variant} />
        <div className="mt-16 border-y border-border py-12 md:py-16">
          <p className="text-xs font-medium tracking-[0.1em] text-muted uppercase">
            Simple on purpose
          </p>
          <h1 className="mt-8 text-[clamp(2.8rem,6.4vw,6.5rem)] leading-[0.9] font-medium tracking-[-0.07em]">
            <span className="block">No plans.</span>
            <span className="block text-muted">No seats.</span>
            <span className="block">No markup.</span>
          </h1>
          <div className="mt-12 grid gap-8 border-t border-border pt-8 md:grid-cols-[1fr_300px] md:items-end">
            <p className="max-w-[700px] text-lg leading-8">
              Pay a 12% fee when you add credits. Everything around your agents
              is unlimited. Agent costs use credits at cost.
            </p>
            <p className="text-right text-6xl font-medium tracking-[-0.06em] md:text-7xl">
              12%
            </p>
          </div>
        </div>
      </section>
    )
  }

  return (
    <section className="mx-auto w-[calc(100%-2rem)] max-w-[1300px] pt-20">
      <VariantNav active={variant} />
      <div className="mt-16 grid items-end gap-12 lg:grid-cols-[1.12fr_0.72fr] lg:gap-20">
        <div>
          <p className="text-xs font-medium tracking-[0.1em] text-muted uppercase">
            No subscriptions. No surprises.
          </p>
          <h1 className="mt-5 max-w-[800px] text-[clamp(2.7rem,5vw,4.8rem)] leading-[0.96] font-medium tracking-[-0.06em] text-balance">
            One fee. Everything else stays simple.
          </h1>
          <p className="mt-7 max-w-[640px] text-base leading-7 text-muted">
            Pay 12% when you add credits. Run unlimited agents, sessions, and
            sandboxes with unlimited knowledge base storage.
          </p>
        </div>
        <div className="rounded-sm border border-border bg-surface p-6 md:p-8">
          <p className="text-xs font-medium tracking-[0.08em] text-muted uppercase">
            The entire Hivy fee
          </p>
          <p className="mt-5 text-7xl font-medium tracking-[-0.07em]">12%</p>
          <p className="mt-5 border-t border-border pt-5 text-sm leading-6 text-muted">
            Charged once when credits are added. Never monthly and never on
            agent costs.
          </p>
        </div>
      </div>
    </section>
  )
}

function UnlimitedSection({ variant }: { variant: PricingVariant }) {
  return (
    <section className="mx-auto mt-32 w-[calc(100%-2rem)] max-w-[1300px]">
      <div
        className={`grid gap-10 ${variant === "receipt" ? "lg:grid-cols-[0.72fr_1.28fr]" : "lg:grid-cols-[0.8fr_1.2fr]"}`}
      >
        <div>
          <p className="text-xs font-medium tracking-[0.08em] text-muted uppercase">
            Included at no extra cost
          </p>
          <h2 className="mt-4 max-w-[520px] text-[clamp(2rem,3.5vw,3.2rem)] leading-[1] font-medium tracking-[-0.05em]">
            Your team can grow without creating a bigger software bill.
          </h2>
        </div>
        <div className="divide-y divide-border border-y border-border">
          {unlimitedItems.map(([title, description], index) => (
            <div
              key={title}
              className={`grid gap-3 py-5 sm:grid-cols-[42px_220px_1fr] sm:items-center ${variant === "unlimited" && index === 1 ? "bg-surface-secondary px-4" : ""}`}
            >
              <span className="text-sm font-medium text-muted">
                0{index + 1}
              </span>
              <h3 className="text-base font-medium">Unlimited {title}</h3>
              <p className="text-sm leading-6 text-muted">{description}</p>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}

function CalculatorSection({ variant }: { variant: PricingVariant }) {
  const headings: Record<PricingVariant, string> = {
    plain: "See the only calculation we make.",
    unlimited: "Add exactly the balance you want.",
    receipt: "Make your own receipt.",
    manifesto: "One input. One fee. Done.",
  }

  return (
    <section className="mx-auto mt-32 w-[calc(100%-2rem)] max-w-[1160px]">
      <p className="text-xs font-medium tracking-[0.08em] text-muted uppercase">
        Deposit calculator
      </p>
      <h2 className="mt-4 max-w-[720px] text-[clamp(2rem,3.5vw,3.2rem)] leading-[1] font-medium tracking-[-0.05em]">
        {headings[variant]}
      </h2>
      <p className="mt-5 max-w-[62ch] text-sm leading-6 text-muted">
        Choose the credit value you want. We add 12% at checkout. There is no
        subscription before or after it.
      </p>
      <div className="mt-10">
        <PricingCalculator mode={variant} />
      </div>
    </section>
  )
}

function AtCostSection({ variant }: { variant: PricingVariant }) {
  const rows = [
    ["Model and provider costs", "Charged at cost"],
    ["Hivy markup on agent costs", "0%"],
    ["Prompt-cache savings", "Passed through"],
    ["Quantized-model savings", "Passed through"],
  ] as const

  return (
    <section className="mx-auto mt-32 w-[calc(100%-2rem)] max-w-[1300px]">
      <div
        className={`grid gap-12 ${variant === "manifesto" ? "border-y border-border py-14 lg:grid-cols-[1.2fr_0.8fr]" : "lg:grid-cols-2"}`}
      >
        <div>
          <p className="text-xs font-medium tracking-[0.08em] text-muted uppercase">
            Agent costs at cost
          </p>
          <h2 className="mt-4 max-w-[580px] text-[clamp(2rem,3.5vw,3.2rem)] leading-[1] font-medium tracking-[-0.05em]">
            We do not make models more expensive.
          </h2>
          <p className="mt-5 max-w-[58ch] text-sm leading-6 text-muted">
            Credits cover the underlying model and provider costs created by
            your agents. If the model gets cheaper, your work gets cheaper.
          </p>
        </div>
        <div className="divide-y divide-border border-y border-border">
          {rows.map(([label, value]) => (
            <div
              key={label}
              className="flex items-center justify-between gap-8 py-4 text-sm"
            >
              <span className="text-muted">{label}</span>
              <span className="text-right font-medium">{value}</span>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}

function PricingFaq() {
  const questions = [
    {
      question: "Is there a monthly subscription?",
      answer:
        "No. Hivy has no monthly subscription and no per-seat plan. Add credits only when you need them.",
    },
    {
      question: "What is unlimited?",
      answer:
        "Agents, sessions, sandboxes, and knowledge base storage are all unlimited at no extra charge.",
    },
    {
      question: "What uses my credits?",
      answer:
        "Credits cover the underlying model and provider costs generated by agent work. Hivy adds 0% markup to those costs.",
    },
    {
      question: "When is the 12% fee charged?",
      answer:
        "Only when you add credits. If you add $100 of credit value, the fee is $12 and you pay $112 at checkout.",
    },
  ]

  return (
    <section className="mx-auto mt-32 w-[calc(100%-2rem)] max-w-[920px]">
      <p className="text-xs font-medium tracking-[0.08em] text-muted uppercase">
        Questions
      </p>
      <h2 className="mt-4 text-[clamp(2rem,3.5vw,3rem)] leading-none font-medium tracking-[-0.05em]">
        The short version.
      </h2>
      <div className="mt-10 divide-y divide-border border-y border-border">
        {questions.map((item) => (
          <details key={item.question} className="group py-5">
            <summary className="focus-visible:ring-ring flex cursor-pointer list-none items-center justify-between gap-6 text-base font-medium outline-none focus-visible:ring-2">
              {item.question}
              <AppIcon
                icon="plus"
                size={17}
                className="shrink-0 transition-transform duration-150 group-open:rotate-45"
              />
            </summary>
            <p className="mt-4 max-w-[68ch] text-sm leading-6 text-muted">
              {item.answer}
            </p>
          </details>
        ))}
      </div>
    </section>
  )
}

function PricingCta() {
  return (
    <section className="mx-auto flex min-h-[400px] w-[calc(100%-2rem)] max-w-[1300px] flex-col items-center justify-center text-center">
      <h2 className="max-w-[760px] text-[clamp(2.3rem,4vw,3.8rem)] leading-[0.98] font-medium tracking-[-0.055em]">
        Start without starting a subscription.
      </h2>
      <p className="mt-5 max-w-[54ch] text-sm leading-6 text-muted">
        Build for free. Add credits when your agents are ready to work.
      </p>
      <div className="mt-7 flex items-center gap-2">
        <Link href="/auth/signup">
          <Button size="sm">Start for free</Button>
        </Link>
        <Link href="mailto:sales@usehivy.com">
          <Button size="sm" variant="ghost">
            Ask about pricing
          </Button>
        </Link>
      </div>
    </section>
  )
}

export function PricingPage({ variant }: { variant: PricingVariant }) {
  const calculatorFirst = variant === "receipt" || variant === "manifesto"

  return (
    <main className="light min-h-screen bg-background text-foreground">
      <LandingHeader />
      <PricingHero variant={variant} />
      {calculatorFirst ? <CalculatorSection variant={variant} /> : null}
      <UnlimitedSection variant={variant} />
      {!calculatorFirst ? <CalculatorSection variant={variant} /> : null}
      <AtCostSection variant={variant} />
      <PricingFaq />
      <PricingCta />
      <LandingFooter />
    </main>
  )
}
