"use client"

import { useRouter } from "next/navigation"
import { Button, Skeleton } from "@heroui/react"
import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"
import { formatCreditsWithUsage } from "./credit-value"

type DashboardResponse = components["schemas"]["dashboardResponse"]

export function CreditsBalanceSection() {
  const router = useRouter()
  const dashboardQuery = $api.useQuery("get", "/v1/dashboard")
  const credits = (dashboardQuery.data as DashboardResponse | undefined)
    ?.credits
  const balance = credits?.balance ?? 0

  return (
    <section className="flex flex-col gap-3">
      <div>
        <h2 className="text-sm font-medium">Credits balance</h2>
        <p className="text-sm text-muted">
          Credits are spent on metered usage. 1,000 credits equals $1 of model
          or sandbox usage.
        </p>
      </div>
      <div className="bg-surface flex items-center gap-4 rounded-2xl border border-border px-4 py-4">
        <div className="flex min-w-0 flex-1 flex-col gap-0.5">
          {dashboardQuery.isLoading ? (
            <>
              <Skeleton className="h-4 w-48 rounded" />
              <Skeleton className="mt-1 h-3.5 w-64 rounded" />
            </>
          ) : (
            <>
              <span className="text-sm font-medium">
                {formatCreditsWithUsage(balance)}
              </span>
              <span className="text-sm text-muted">
                Current balance for this workspace
              </span>
            </>
          )}
        </div>
        <Button
          variant="tertiary"
          size="sm"
          isDisabled={dashboardQuery.isLoading}
          onPress={() => router.push("/w/billing/plans")}
        >
          Buy credits
        </Button>
      </div>
    </section>
  )
}
