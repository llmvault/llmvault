import Link from "next/link"
import { Button } from "@heroui/react"
import { YourPlanSection } from "./_components/your-plan-section"
import { CreditsUsageSection } from "./_components/credits-usage-section"

export default function BillingSettingsPage() {
  return (
    <div className="flex flex-col gap-10">
      <div className="flex flex-col gap-2">
        <h1 className="text-2xl font-semibold">Usage &amp; billing</h1>
        <p className="text-sm text-muted">
          To view invoices, change your payment method, and take other actions,
          visit{" "}
          <Link
            href="/w/credits"
            className="text-accent underline-offset-2 hover:underline"
          >
            settings
          </Link>{" "}
          on web.
        </p>
      </div>

      <YourPlanSection />

      <section className="flex flex-col gap-3">
        <div>
          <h2 className="text-sm font-medium">Credits balance</h2>
          <p className="text-sm text-muted">
            Buy credits or turn on auto-reload to continue using Hivy if you hit
            a limit.{" "}
            <a
              href="#"
              className="text-accent underline-offset-2 hover:underline"
            >
              Learn more
            </a>
          </p>
        </div>
        <div className="flex items-center gap-4 rounded-2xl border border-border bg-surface px-4 py-4">
          <div className="flex min-w-0 flex-1 flex-col gap-0.5">
            <span className="text-sm font-medium">NGN 0</span>
            <span className="text-sm text-muted">
              Current balance ·{" "}
              <a
                href="#"
                className="text-accent underline-offset-2 hover:underline"
              >
                Manage auto-reload
              </a>
            </span>
          </div>
          <Button variant="tertiary" size="sm">
            Buy credits
          </Button>
        </div>
      </section>

      <CreditsUsageSection />

      <section className="flex flex-col gap-2">
        <h2 className="text-sm font-medium">Cancel plan</h2>
        <p className="text-sm text-muted">
          Your subscription is managed through Hivy. Go to{" "}
          <Link
            href="/w/credits"
            className="text-accent underline-offset-2 hover:underline"
          >
            billing
          </Link>{" "}
          to cancel your plan.
        </p>
      </section>
    </div>
  )
}
