"use client"

import { Skeleton } from "@heroui/react"
import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"

type DashboardResponse = components["schemas"]["dashboardResponse"]

function formatNumber(value: number | undefined | null) {
  if (value == null) return "0"
  return value.toLocaleString("en-NG")
}

function formatDate(value: string | undefined | null) {
  if (!value) return "—"
  return new Intl.DateTimeFormat("en-NG", {
    day: "numeric",
    month: "short",
    year: "numeric",
  }).format(new Date(value))
}

export function CreditsUsageSection() {
  const dashboardQuery = $api.useQuery("get", "/v1/dashboard")
  const credits = (dashboardQuery.data as DashboardResponse | undefined)
    ?.credits

  const spent = credits?.spent_this_period ?? 0
  const balance = credits?.balance ?? 0
  const isLoading = dashboardQuery.isLoading

  return (
    <section className="flex flex-col gap-3">
      <div>
        <h2 className="text-sm font-medium">Usage this month</h2>
        <p className="text-sm text-muted">
          Purchased credits do not reset. This view groups spending by calendar
          month.
        </p>
      </div>
      <div className="rounded-2xl border border-border bg-surface p-5">
        {isLoading ? (
          <Skeleton className="h-16" />
        ) : (
          <>
            <div className="grid grid-cols-2 gap-6">
              <div>
                <p className="text-xs text-muted">Spent this month</p>
                <p className="mt-1 text-2xl font-semibold tabular-nums">
                  {formatNumber(spent)}
                </p>
              </div>
              <div>
                <p className="text-xs text-muted">Available balance</p>
                <p className="mt-1 text-2xl font-semibold tabular-nums">
                  {formatNumber(balance)}
                </p>
              </div>
            </div>
            {credits?.period_start && credits?.period_end ? (
              <div className="mt-4 flex justify-between border-t border-border pt-3 text-xs text-muted">
                <span>
                  {formatDate(credits.period_start)} –{" "}
                  {formatDate(credits.period_end)}
                </span>
                <span>Calendar month</span>
              </div>
            ) : null}
          </>
        )}
      </div>
    </section>
  )
}
