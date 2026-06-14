"use client"

import { ProgressBar } from "@heroui/react"
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
  const credits = (dashboardQuery.data as DashboardResponse | undefined)?.credits

  const spent = credits?.spent_this_period ?? 0
  const balance = credits?.balance ?? 0
  const total = spent + balance
  const pctUsed = total > 0 ? Math.round((spent / total) * 100) : 0
  const isLoading = dashboardQuery.isLoading

  return (
    <section className="flex flex-col gap-3">
      <div>
        <h2 className="text-sm font-medium">Usage this period</h2>
        <p className="text-sm text-muted">
          {credits?.period_end
            ? `Your included credits reset on ${formatDate(credits.period_end)}.`
            : "Track the credits you've used this billing period."}
        </p>
      </div>
      <div className="rounded-2xl border border-border bg-surface p-5">
        {isLoading ? (
          <div className="h-16 animate-pulse rounded-xl bg-default" />
        ) : (
          <>
            <div className="flex flex-wrap items-end justify-between gap-2">
              <span className="text-2xl font-semibold tabular-nums">
                {formatNumber(spent)}
                <span className="text-sm font-normal text-muted">
                  {" "}
                  / {formatNumber(total)} credits spent
                </span>
              </span>
              <span className="text-sm text-muted">
                {formatNumber(balance)} left
              </span>
            </div>
            <ProgressBar
              value={pctUsed}
              aria-label="Credits used this period"
              className="mt-4"
            >
              <ProgressBar.Track>
                <ProgressBar.Fill />
              </ProgressBar.Track>
            </ProgressBar>
            {credits?.period_start && credits?.period_end ? (
              <div className="mt-2 flex justify-between text-xs text-muted">
                <span>
                  {formatDate(credits.period_start)} –{" "}
                  {formatDate(credits.period_end)}
                </span>
                <span>{pctUsed}% used</span>
              </div>
            ) : null}
          </>
        )}
      </div>
    </section>
  )
}
