"use client"

import { useMemo } from "react"
import NextLink from "next/link"
import { AppIcon } from "@/components/icon"
import { $api } from "@/lib/api/hooks"
import { AutomationsListView } from "@/app/w/(chat)/automations/_automation-list"
import { automationFromCatalog } from "@/app/w/(chat)/automations/_data"
import { TutorialBanner } from "@/components/tutorial-banner"

export default function NewTriggerPage() {
  const triggersQuery = $api.useQuery("get", "/v1/catalog/triggers")
  const automations = useMemo(
    () =>
      (triggersQuery.data?.data ?? []).map((item) =>
        automationFromCatalog(
          item,
          "Triggers",
          `/w/automations/${item.slug || ""}/install`
        )
      ),
    [triggersQuery.data?.data]
  )

  return (
    <AutomationsListView
      nav={
        <NextLink
          href="/w/automations"
          className="text-muted-foreground hover:text-foreground flex w-fit items-center gap-1.5 text-sm transition-colors"
        >
          <AppIcon icon="arrow-left" className="h-4 w-4" />
          Automations
        </NextLink>
      }
      automations={automations}
      isLoading={triggersQuery.isLoading}
      isError={triggersQuery.isError}
      onRetry={() => void triggersQuery.refetch()}
      title="Install triggers"
      description="Start agent work from connection events"
      searchLabel="triggers"
      emptyTab="Triggers"
      tutorial={
        <TutorialBanner
          tutorial="automations"
          title="Install your first connection trigger"
          description="See how a connected app event becomes a reliable agent workflow."
          docsPath="automations/event-triggers"
        />
      }
    />
  )
}
