"use client"

import { useEffect } from "react"
import { useRouter } from "next/navigation"
import { useAuth } from "@/lib/auth/auth-context"
import { useIsAdmin } from "@/lib/auth/use-role"
import { CreditsBalanceSection } from "./_components/credits-balance-section"
import { CreditsUsageSection } from "./_components/credits-usage-section"
import { CreditPurchasesSection } from "./_components/credit-purchases-section"

export default function BillingPageContent() {
  const router = useRouter()
  const isAdmin = useIsAdmin()
  const { isLoading } = useAuth()

  useEffect(() => {
    if (!isLoading && !isAdmin) router.replace("/w/teams")
  }, [isLoading, isAdmin, router])

  if (!isAdmin) return null

  return (
    <div className="mx-auto flex w-full max-w-2xl flex-col gap-10">
      <div className="flex flex-col gap-2">
        <h1 className="text-2xl font-semibold">Usage &amp; billing</h1>
        <p className="text-sm text-muted">
          Buy credits and track usage for this workspace.
        </p>
      </div>

      <CreditsBalanceSection
        onBuyCredits={() => router.push("/w/billing/purchase")}
      />

      <CreditPurchasesSection />

      <CreditsUsageSection />
    </div>
  )
}
