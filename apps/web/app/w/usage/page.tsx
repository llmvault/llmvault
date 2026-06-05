"use client"

import { useState, useMemo } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Chart01Icon,
  CreditCardIcon,
  MessageMultiple01Icon,
} from "@hugeicons/core-free-icons"
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
} from "recharts"
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
} from "@/components/ui/chart"
import { Card } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { DateRangePicker } from "@/components/ui/date-range-picker"
import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"

type UsageResponse = components["schemas"]["usageResponse"]
type SubscriptionResponse = components["schemas"]["subscriptionResponse"]

function formatNumber(n: number | undefined | null): string {
  if (n == null) return "—"
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return n.toLocaleString()
}

function formatCredits(n: number | undefined | null): string {
  if (n == null) return "—"
  return n.toFixed(2)
}

const chartConfig = {
  credits: {
    label: "Credits",
    color: "var(--primary)",
  },
}

function MetricCard({
  icon,
  label,
  value,
  sub,
}: {
  icon: typeof Chart01Icon
  label: string
  value: string
  sub?: string
}) {
  return (
    <Card className="flex items-center gap-4 p-5">
      <div className="flex size-11 shrink-0 items-center justify-center rounded-lg bg-primary/10">
        <HugeiconsIcon icon={icon} className="size-5 text-primary" />
      </div>
      <div className="min-w-0 flex-1">
        <p className="text-xs text-muted-foreground">{label}</p>
        <p className="mt-0.5 font-heading text-2xl font-medium tracking-[-0.02em] text-foreground">
          {value}
        </p>
        {sub && (
          <p className="mt-0.5 text-xs text-muted-foreground">{sub}</p>
        )}
      </div>
    </Card>
  )
}

export default function UsagePage() {
  const [activeTab, setActiveTab] = useState("overview")
  const [selectedSession, setSelectedSession] = useState<string | null>(null)

  const today = useMemo(() => {
    const d = new Date()
    const y = d.getFullYear()
    const m = String(d.getMonth() + 1).padStart(2, "0")
    const day = String(d.getDate()).padStart(2, "0")
    return `${y}-${m}-${day}`
  }, [])
  const defaultStart = useMemo(() => {
    const d = new Date(today + "T00:00:00")
    d.setDate(d.getDate() - 30)
    const y = d.getFullYear()
    const m = String(d.getMonth() + 1).padStart(2, "0")
    const day = String(d.getDate()).padStart(2, "0")
    return `${y}-${m}-${day}`
  }, [today])

  const [startDate, setStartDate] = useState(defaultStart)
  const [endDate, setEndDate] = useState(today)

  const usageQuery = $api.useQuery("get", "/v1/usage", {
    params: {
      query: {
        start_date: startDate,
        end_date: endDate,
      },
    },
  })
  const billingQuery = $api.useQuery("get", "/v1/billing/subscription")

  const usage = usageQuery.data as UsageResponse | undefined
  const billing = billingQuery.data as SubscriptionResponse | undefined
  const isLoading = usageQuery.isLoading || billingQuery.isLoading

  const spendData = usage?.spend_over_time ?? []
  const sessions = usage?.sessions ?? []
  const displaySessions = useMemo(
    () =>
      sessions.length > 0
        ? sessions
        : [
            {
              id: "mock-1",
              name: "Code review for PR #142",
              status: "ended",
              source: "slack",
              event_count: 24,
              created_at: new Date(Date.now() - 3600000).toISOString(),
            },
            {
              id: "mock-2",
              name: "Debug production incident",
              status: "ended",
              source: "web",
              event_count: 47,
              created_at: new Date(Date.now() - 86400000).toISOString(),
            },
            {
              id: "mock-3",
              name: "Research competitor features",
              status: "active",
              source: "slack",
              event_count: 12,
              created_at: new Date(Date.now() - 1800000).toISOString(),
            },
            {
              id: "mock-4",
              name: "Draft Q2 engineering doc",
              status: "ended",
              source: "web",
              event_count: 31,
              created_at: new Date(Date.now() - 172800000).toISOString(),
            },
            {
              id: "mock-5",
              name: "",
              status: "ended",
              source: "api",
              event_count: 3,
              created_at: new Date(Date.now() - 259200000).toISOString(),
            },
          ],
    [sessions],
  )

  const dayCount = useMemo(() => {
    const s = new Date(startDate)
    const e = new Date(endDate)
    return Math.max(1, Math.round((e.getTime() - s.getTime()) / 86400000) + 1)
  }, [startDate, endDate])

  const totalCredits = useMemo(
    () => spendData.reduce((sum, d) => sum + (d.total_cost ?? 0), 0),
    [spendData],
  )
  const burnRate = totalCredits / dayCount
  const creditsBalance = billing?.credits_balance ?? 0

  const tabs = [
    { id: "overview", label: "Overview" },
    { id: "sessions", label: "Sessions" },
  ]

  return (
    <div className="mx-auto w-full max-w-5xl flex flex-1 flex-col gap-8">
      {/* Header */}
      <div>
        <h1 className="font-heading text-3xl font-normal tracking-[-0.02em] text-foreground">
          Usage
        </h1>
        <p className="mt-3 text-sm text-muted-foreground">
          Monitor API usage, credits, and activity across your workspace.
        </p>
      </div>

      {/* Tabs */}
      <div className="flex gap-1 border-b border-border">
        {tabs.map((tab) => (
          <button
            key={tab.id}
            onClick={() => setActiveTab(tab.id)}
            className={`relative px-4 pb-3 text-sm font-medium transition-colors ${
              activeTab === tab.id
                ? "text-foreground after:absolute after:bottom-0 after:left-0 after:h-0.5 after:w-full after:bg-foreground"
                : "text-muted-foreground hover:text-foreground"
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {isLoading ? (
        <div className="flex flex-col gap-6">
          <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} className="h-28 rounded-xl" />
            ))}
          </div>
          <Skeleton className="h-72 rounded-xl" />
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
            <Skeleton className="h-48 rounded-xl" />
            <Skeleton className="h-48 rounded-xl" />
          </div>
        </div>
      ) : (
        <>
          {/* Overview Tab */}
          {activeTab === "overview" && (
            <div className="flex flex-col gap-8">
              {/* Summary cards */}
              <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
                <MetricCard
                  icon={Chart01Icon}
                  label="Total credits"
                  value={formatCredits(totalCredits)}
                  sub={`in selected period`}
                />
                <MetricCard
                  icon={Chart01Icon}
                  label="Daily average"
                  value={formatCredits(burnRate)}
                  sub={`Avg credits per day (${dayCount}d)`}
                />
                <MetricCard
                  icon={CreditCardIcon}
                  label="Credit balance"
                  value={formatNumber(creditsBalance)}
                  sub="Available credits"
                />
              </div>

              {/* Daily credits chart */}
              <div className="rounded-xl border border-border bg-card p-6">
                <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
                  <div>
                    <h2 className="font-heading text-lg font-medium text-foreground">
                      Daily credits
                    </h2>
                    <p className="mt-1 text-sm text-muted-foreground">
                      Credit consumption over the selected period.
                    </p>
                  </div>
                  <DateRangePicker
                    startDate={startDate}
                    endDate={endDate}
                    onChange={(s, e) => {
                      setStartDate(s)
                      setEndDate(e)
                    }}
                  />
                </div>

                {spendData.length > 0 ? (
                  <ChartContainer config={chartConfig} className="aspect-[3/1]">
                    <BarChart data={spendData} margin={{ top: 8, right: 8, bottom: 0, left: -16 }}>
                      <CartesianGrid
                        vertical={false}
                        stroke="hsl(var(--border))"
                        strokeDasharray="3 3"
                      />
                      <XAxis
                        dataKey="date"
                        tickLine={false}
                        axisLine={false}
                        tickMargin={8}
                        tickFormatter={(v: string) => {
                          const d = new Date(v)
                          return `${d.getMonth() + 1}/${d.getDate()}`
                        }}
                        fontSize={11}
                      />
                      <YAxis
                        tickLine={false}
                        axisLine={false}
                        tickMargin={8}
                        fontSize={11}
                        tickFormatter={(v: number) => v.toFixed(2)}
                      />
                      <ChartTooltip
                        content={
                          <ChartTooltipContent
                            indicator="dot"
                            labelFormatter={(label: any) => {
                              const d = new Date(String(label))
                              return d.toLocaleDateString("en-US", {
                                weekday: "short",
                                month: "short",
                                day: "numeric",
                              })
                            }}
                          />
                        }
                        cursor={{ fill: "hsl(var(--muted))", opacity: 0.3 }}
                      />
                      <Bar
                        dataKey="total_cost"
                        fill="var(--color-credits)"
                        radius={[3, 3, 0, 0]}
                        maxBarSize={32}
                      />
                    </BarChart>
                  </ChartContainer>
                ) : (
                  <div className="flex h-48 items-center justify-center rounded-lg border border-dashed border-border">
                    <p className="text-sm text-muted-foreground">
                      No credit data in this period
                    </p>
                  </div>
                )}
              </div>
            </div>
          )}

          {/* Team Tab */}
          {/* Sessions Tab */}
          {activeTab === "sessions" && (
            <div className="flex flex-col gap-6">
              <div>
                <h2 className="font-heading text-lg font-medium text-foreground">
                  AI employee sessions
                </h2>
                <p className="mt-1 text-sm text-muted-foreground">
                  Past sessions and conversations with Hivy.
                </p>
              </div>

              {selectedSession ? (
                <div className="flex flex-col gap-4">
                  <button
                    onClick={() => setSelectedSession(null)}
                    className="self-start text-sm text-muted-foreground hover:text-foreground"
                  >
                    &larr; Back to sessions
                  </button>
                  <div className="rounded-xl border border-border bg-card p-6">
                    <pre className="text-sm text-foreground">
                      {JSON.stringify(
                        displaySessions.find((s) => s.id === selectedSession),
                        null,
                        2,
                      )}
                    </pre>
                  </div>
                </div>
              ) : (
                <div className="rounded-xl border border-border bg-card divide-y divide-border">
                  {displaySessions.map((session) => (
                    <button
                      key={session.id}
                      onClick={() => setSelectedSession(session.id ?? null)}
                      className="flex w-full items-center gap-4 px-6 py-4 text-left hover:bg-muted/50 transition-colors"
                    >
                      <div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary/10">
                        <HugeiconsIcon icon={MessageMultiple01Icon} className="size-4 text-primary" />
                      </div>
                      <div className="min-w-0 flex-1">
                        <p className="truncate text-sm font-medium text-foreground">
                          {session.name || "Untitled session"}
                        </p>
                        <p className="mt-0.5 text-xs text-muted-foreground">
                          {session.source && `${session.source} · `}
                          {session.event_count != null && `${session.event_count} event${session.event_count !== 1 ? "s" : ""} · `}
                          {session.created_at
                            ? new Date(session.created_at).toLocaleDateString("en-US", {
                                month: "short",
                                day: "numeric",
                                hour: "numeric",
                                minute: "2-digit",
                              })
                            : ""}
                        </p>
                      </div>
                      <span
                        className={`shrink-0 rounded-full px-2.5 py-0.5 text-xs font-medium ${
                          session.status === "active"
                            ? "bg-green-500/10 text-green-600"
                            : session.status === "ended"
                              ? "bg-muted text-muted-foreground"
                              : "bg-red-500/10 text-red-600"
                        }`}
                      >
                        {session.status ?? "unknown"}
                      </span>
                    </button>
                  ))}
                </div>
              )}
            </div>
          )}
        </>
      )}
    </div>
  )
}
