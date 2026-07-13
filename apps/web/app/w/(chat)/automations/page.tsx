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
import { TutorialBanner } from "@/components/tutorial-banner"

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
        tutorial={<AutomationTutorial kind="schedules" />}
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
        tutorial={<AutomationTutorial kind="webhooks" />}
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
      tutorial={<AutomationTutorial kind="automations" />}
    />
  )
}

function AutomationTutorial({
  kind,
}: {
  kind: "automations" | "schedules" | "webhooks"
}) {
  const content = {
    automations: {
      title: "Build your first connection automation",
      description: "See how an event in a connected app can start agent work.",
      path: "automations/connections",
    },
    schedules: {
      title: "Schedule recurring agent work",
      description: "Learn how to choose an agent, set a cadence, and write the task.",
      path: "automations/schedules",
    },
    webhooks: {
      title: "Trigger an agent with a webhook",
      description: "Follow a request from its unique URL through to an agent run.",
      path: "automations/webhooks",
    },
  }[kind]
  return (
    <TutorialBanner
      tutorial={kind}
      title={content.title}
      description={content.description}
      docsPath={content.path}
    />
  )
}

function tabFromParam(raw: string | null): AutomationsTab {
  if (raw === "schedules" || raw === "webhooks") return raw
  return "connections"
}
