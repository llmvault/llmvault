import { YourPlanSection } from "./_components/your-plan-section"
import { CreditsBalanceSection } from "./_components/credits-balance-section"
import { CreditsUsageSection } from "./_components/credits-usage-section"
import { CancelPlanSection } from "./_components/cancel-plan-section"

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

      <CancelPlanSection />
    </div>
  )
}
