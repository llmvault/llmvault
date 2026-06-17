import { BillingPlansPage } from "@/app/w/settings/billing/_components/plans-page"
import { AuthProvider } from "@/lib/auth/auth-context"

export default function BillingPlansRoute() {
  return (
    <AuthProvider>
      <BillingPlansPage />
    </AuthProvider>
  )
}
