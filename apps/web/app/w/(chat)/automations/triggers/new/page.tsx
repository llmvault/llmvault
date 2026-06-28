"use client"

import { useMemo } from "react"
import { $api } from "@/lib/api/hooks"
import { AutomationsListView } from "@/app/w/(chat)/automations/_automation-list"
import { automationFromCatalog } from "@/app/w/(chat)/automations/_data"

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
      automations={automations}
      isLoading={triggersQuery.isLoading}
      isError={triggersQuery.isError}
      onRetry={() => void triggersQuery.refetch()}
      searchLabel="triggers"
      emptyTab="Triggers"
    />
  )
}
