import Link from "next/link"
import type { ReactNode } from "react"
import { AppIcon } from "@/components/icon"
import { DocsMediaPlaceholder } from "./docs-media-placeholder"

const USAGE_VIEWS = [
  {
    icon: "coins" as const,
    title: "Credits left",
    description:
      "Read the shared balance and the amount spent since the current billing period began.",
  },
  {
    icon: "calendar" as const,
    title: "This month's usage",
    description:
      "Usage is grouped by calendar month. Purchased credits stay in the workspace until they are spent.",
  },
  {
    icon: "messages-square" as const,
    title: "Cost per session",
    description:
      "Open any session you can access to check its credits and estimated dollar cost.",
  },
]

export function UsageBilling() {
  return (
    <div className="mt-10 text-base leading-7">
      <p className="max-w-2xl text-muted">
        Every Hivy workspace has one credit balance. Agents spend from it when
        Hivy pays for model or sandbox work, and the model you choose can change
        the price of the same task by a lot. Check the total before routine work
        quietly eats through the month.
      </p>

      <section aria-labelledby="how-credits-work" className="mt-14">
        <h2
          id="how-credits-work"
          className="text-xl font-semibold tracking-tight text-foreground"
        >
          Know what a credit buys
        </h2>
        <p className="mt-3 max-w-2xl text-muted">
          One credit pays for $0.001 of metered usage; 1,000 credits covers $1
          of work. Hivy charges the listed model cost when it supplies the model
          credential, while sandbox work follows its published rate; more
          context and extra model turns raise the session total.
        </p>
        <div className="mt-7 divide-y divide-border overflow-hidden rounded-xl border border-border bg-surface">
          {USAGE_VIEWS.map((item) => (
            <div key={item.title} className="p-5">
              <AppIcon icon={item.icon} className="h-4 w-4 text-accent" />
              <h3 className="mt-4 font-semibold text-foreground">
                {item.title}
              </h3>
              <p className="mt-2 text-sm leading-6 text-muted">
                {item.description}
              </p>
            </div>
          ))}
        </div>
      </section>

      <DocsMediaPlaceholder
        type="video"
        title="Buy credits through Paystack"
        description="Use a demo owner account to open Settings > Usage & billing, choose a preset or enter a custom deposit amount, compare its 12% fee and total, then select a saved card or a new card. Keep payment details out of frame."
        className="mt-12"
      />

      <div className="mt-16 space-y-14 border-t border-border pt-14">
        <DocSection title="Find the workspace total">
          <p>
            Go to <strong className="text-foreground">Settings</strong> and open{" "}
            <strong className="text-foreground">Usage &amp; billing</strong>.
            You&apos;ll find the remaining credits, this month&apos;s spend, and
            recent purchases on one page.
          </p>
          <p className="mt-3">
            That balance belongs to the whole workspace, not one team. Check it
            before rolling an agent out more widely or assigning routine work to
            a pricier model.
          </p>
          <DocLink href="/w/billing">Open Usage &amp; billing</DocLink>
        </DocSection>

        <DocSection title="Read a session price">
          <p>
            Once the agent finishes, look at the session composer. Its footer
            reports the credits Hivy charged and their estimated dollar value,
            which gives you a clean way to compare models on the same job.
          </p>
          <p className="mt-3">
            Members get this local cost only for sessions their team access lets
            them open; Hivy shows the workspace bill only to admins and the
            owner.
          </p>
          <DocLink href="/docs/agents/sessions">
            Read about agent sessions
          </DocLink>
        </DocSection>

        <DocsMediaPlaceholder
          type="image"
          title="Where a completed session shows its cost"
          description="Use a finished demo session with no sensitive information. At 4K and 100% browser zoom, crop close enough to read the model selector, credit count, dollar cost, and final result without opening the image."
          bleed={false}
        />

        <DocSection title="Buy the credits you need">
          <p>
            New workspaces receive 1,000 welcome credits once. When you need
            more, the owner chooses a fixed pack. Hivy shows the credits, 12%
            deposit fee, and final Paystack charge before checkout.
          </p>
          <p className="mt-3">
            USD packs start at $5. The matching NGN pack is ₦7,250 at
            Hivy&apos;s fixed ₦1,450 per USD credit conversion. Run the tasks
            your team repeats and compare their session totals before choosing a
            pack.
          </p>
        </DocSection>

        <DocSection title="Reuse a card without storing it in Hivy">
          <p>
            Pay with a new card and choose whether Paystack should save the
            payment authorization. On a later purchase, the owner can select a
            saved card in the same currency or remove it from the workspace
            billing page.
          </p>
          <p className="mt-3">
            A pending purchase remains in Recent purchases. The owner can ask
            Hivy to verify it after Paystack finishes processing the payment.
          </p>
        </DocSection>

        <DocSection title="Choose a currency for each deposit">
          <p>
            The owner can pay in USD or NGN on each purchase. Saved cards remain
            tied to the currency in which Paystack authorized them.
          </p>
          <p className="mt-3">
            Purchases are one-time deposits, not recurring charges. Credits are
            added only after Paystack confirms the exact amount and currency.
          </p>
        </DocSection>

        <DocSection title="Lifetime deposits unlock permanent capacity">
          <p>
            Capacity depends on the total credits the workspace has purchased,
            not the current balance. Reaching a threshold permanently unlocks
            more simultaneous agent sessions, a larger maximum sandbox, and more
            indexed knowledge storage.
          </p>
          <div className="mt-6 overflow-hidden rounded-xl border border-border bg-surface">
            <div className="grid grid-cols-[4.5rem_1fr] gap-4 border-b border-border px-4 py-3 text-sm">
              <strong className="text-foreground">Start</strong>
              <span className="text-muted">
                1 session, Nano sandbox, 1 GB knowledge
              </span>
            </div>
            <div className="grid grid-cols-[4.5rem_1fr] gap-4 border-b border-border px-4 py-3 text-sm">
              <strong className="text-foreground">$100</strong>
              <span className="text-muted">
                2 sessions, Small sandbox, 3 GB knowledge
              </span>
            </div>
            <div className="grid grid-cols-[4.5rem_1fr] gap-4 border-b border-border px-4 py-3 text-sm">
              <strong className="text-foreground">$250</strong>
              <span className="text-muted">
                5 sessions, Medium sandbox, 5 GB knowledge
              </span>
            </div>
            <div className="grid grid-cols-[4.5rem_1fr] gap-4 px-4 py-3 text-sm">
              <strong className="text-foreground">$500</strong>
              <span className="text-muted">
                10 sessions, Large sandbox, 10 GB knowledge
              </span>
            </div>
          </div>
          <p className="mt-4">
            Thresholds use the USD value of completed deposits, including NGN
            purchases. Spending credits or receiving a refund never lowers an
            unlocked capacity tier.
          </p>
        </DocSection>

        <section
          aria-labelledby="billing-access"
          className="rounded-xl border border-border bg-surface-secondary p-6"
        >
          <h2
            id="billing-access"
            className="text-lg font-semibold tracking-tight text-foreground"
          >
            Who gets billing access
          </h2>
          <ul className="mt-4 space-y-3 text-sm leading-6 text-muted">
            <AccessItem>
              Owners and admins can read the workspace balance, purchases, and
              usage.
            </AccessItem>
            <AccessItem>
              Money stays with the owner: only that person can buy or verify
              credits and manage saved cards.
            </AccessItem>
            <AccessItem>
              Members see costs inside the sessions they can already open.
            </AccessItem>
          </ul>
        </section>
      </div>
    </div>
  )
}

function AccessItem({ children }: { children: ReactNode }) {
  return (
    <li className="flex gap-3">
      <AppIcon icon="check" className="mt-1 h-4 w-4 shrink-0 text-accent" />
      <span>{children}</span>
    </li>
  )
}

function DocSection({
  title,
  children,
}: {
  title: string
  children: ReactNode
}) {
  const id = "billing-" + title.toLowerCase().replaceAll(" ", "-")

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
      className="mt-4 inline-flex rounded-sm font-medium text-foreground underline decoration-border underline-offset-4 transition-colors hover:text-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus"
    >
      {children}
    </Link>
  )
}
