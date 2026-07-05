"use client"

import { useMemo, useState } from "react"
import { $api } from "@/lib/api/hooks"
import { AutomationsListView } from "@/app/w/(chat)/automations/_automation-list"
import {
  AutomationPlaceholderView,
  AutomationsTabs,
  type AutomationsTab,
} from "@/app/w/(chat)/automations/_automation-tabs"
import { automationFromInstalledTrigger } from "@/app/w/(chat)/automations/_data"

export default function AutomationsPage() {
  const [tab, setTab] = useState<AutomationsTab>("connections")
  const triggersQuery = $api.useQuery("get", "/v1/triggers")
  const automations = useMemo(
    () =>
      (triggersQuery.data?.data ?? []).map((trigger) =>
        automationFromInstalledTrigger(trigger)
      ),
    [triggersQuery.data?.data]
  )
  const nav = <AutomationsTabs active={tab} onChange={setTab} />

  if (tab === "schedules") {
    return (
      <AutomationPlaceholderView
        nav={nav}
        title="Schedules"
        description="Run agents on a recurring schedule"
        searchLabel="schedules"
        icon="calendar"
        emptyTitle="No schedules yet"
        emptyBody="Recurring agent runs will appear here."
      />
    )
  }

  if (tab === "webhooks") {
    return (
      <AutomationPlaceholderView
        nav={nav}
        title="Webhooks"
        description="Trigger agents from inbound HTTP requests"
        searchLabel="webhooks"
        icon="globe"
        emptyTitle="No webhooks yet"
        emptyBody="HTTP-based triggers will appear here."
      />
    )
  }

  return (
    <AutomationsListView
      nav={nav}
      automations={automations}
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
