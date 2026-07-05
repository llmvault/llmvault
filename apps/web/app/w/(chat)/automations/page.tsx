"use client"

import { Suspense, useMemo, useState } from "react"
import { useSearchParams } from "next/navigation"
import { $api } from "@/lib/api/hooks"
import { AutomationsListView } from "@/app/w/(chat)/automations/_automation-list"
import {
  AutomationsTabs,
  type AutomationsTab,
} from "@/app/w/(chat)/automations/_automation-tabs"
import {
  automationFromInstalledTrigger,
  automationFromSchedule,
  automationFromWebhookTrigger,
} from "@/app/w/(chat)/automations/_data"

export default function AutomationsPage() {
  return (
    <Suspense fallback={null}>
      <AutomationsPageInner />
    </Suspense>
  )
}

function AutomationsPageInner() {
  const searchParams = useSearchParams()
  const [tab, setTab] = useState<AutomationsTab>(
    tabFromParam(searchParams.get("tab"))
  )
  const triggersQuery = $api.useQuery("get", "/v1/triggers")
  const triggers = triggersQuery.data?.data
  const schedulesQuery = $api.useQuery("get", "/v1/schedules")
  const schedules = useMemo(
    () => (schedulesQuery.data?.data ?? []).map(automationFromSchedule),
    [schedulesQuery.data?.data]
  )

  const connections = useMemo(
    () =>
      (triggers ?? [])
        .filter((trigger) => trigger.trigger_type !== "http")
        .map(automationFromInstalledTrigger),
    [triggers]
  )
  const webhooks = useMemo(
    () =>
      (triggers ?? [])
        .filter((trigger) => trigger.trigger_type === "http")
        .map(automationFromWebhookTrigger),
    [triggers]
  )
  const nav = <AutomationsTabs active={tab} onChange={setTab} />

  if (tab === "schedules") {
    return (
      <AutomationsListView
        nav={nav}
        automations={schedules}
        isLoading={schedulesQuery.isLoading}
        isError={schedulesQuery.isError}
        onRetry={() => void schedulesQuery.refetch()}
        action={{
          label: "Add schedule",
          href: "/w/automations/schedules/new",
        }}
        title="Schedules"
        description="Run agents on a recurring schedule"
        searchLabel="schedules"
        emptyTab="Schedules"
      />
    )
  }

  if (tab === "webhooks") {
    return (
      <AutomationsListView
        nav={nav}
        automations={webhooks}
        isLoading={triggersQuery.isLoading}
        isError={triggersQuery.isError}
        onRetry={() => void triggersQuery.refetch()}
        action={{
          label: "Add webhook trigger",
          href: "/w/automations/webhooks/new",
        }}
        title="Webhooks"
        description="Trigger agents from inbound HTTP requests"
        searchLabel="webhooks"
        emptyTab="Webhooks"
      />
    )
  }

  return (
    <AutomationsListView
      nav={nav}
      automations={connections}
      isLoading={triggersQuery.isLoading}
      isError={triggersQuery.isError}
      onRetry={() => void triggersQuery.refetch()}
      action={{
        label: "Install trigger",
        href: "/w/automations/triggers/new",
      }}
      title="Connections"
      description="Run agents when events happen in your connected apps"
      searchLabel="triggers"
      emptyTab="Triggers"
    />
  )
}

function tabFromParam(raw: string | null): AutomationsTab {
  if (raw === "schedules" || raw === "webhooks") return raw
  return "connections"
}
