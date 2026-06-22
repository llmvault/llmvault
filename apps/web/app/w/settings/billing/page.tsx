import { YourPlanSection } from "./_components/your-plan-section"
import { CreditsBalanceSection } from "./_components/credits-balance-section"
import { CreditsUsageSection } from "./_components/credits-usage-section"

export default function BillingSettingsPage() {
  return (
    <div className="flex flex-col gap-10">
      <div className="flex flex-col gap-2">
        <h1 className="text-2xl font-semibold">Usage &amp; billing</h1>
        <p className="text-sm text-muted">
          Manage your plan, credit balance, and usage for this workspace.
        </p>
      </div>

      <YourPlanSection />

      <CreditsBalanceSection />

      <CreditsUsageSection />

      <section className="flex flex-col gap-2">
        <h2 className="text-sm font-medium">Cancel plan</h2>
        <p className="text-sm text-muted">
          Need to cancel your subscription? Contact{" "}
          <a
            href="mailto:hello@usehivy.com"
            className="text-accent underline-offset-2 hover:underline"
          >
            support
          </a>{" "}
          and we&apos;ll take care of it.
        </p>
      </section>
    </div>
  )
}
