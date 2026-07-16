"use client"

import { Button, Skeleton, Tooltip } from "@heroui/react"
import { $api } from "@/lib/api/hooks"
import { useIsOwner } from "@/lib/auth/use-role"
import type { components } from "@/lib/api/schema"
import { formatCreditsWithUsage } from "./credit-value"

type DashboardResponse = components["schemas"]["dashboardResponse"]

export function CreditsBalanceSection() {
  const isOwner = useIsOwner()
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
      <div className="flex items-center gap-4 rounded-2xl border border-border bg-surface px-4 py-4">
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
        <BuyCreditsButton
          isLoading={dashboardQuery.isLoading}
          isOwner={isOwner}
          onPress={() =>
            document
              .getElementById("credit-purchases")
              ?.scrollIntoView({ behavior: "smooth", block: "start" })
          }
        />
      </div>
    </section>
  )
}

function BuyCreditsButton({
  isLoading,
  isOwner,
  onPress,
}: {
  isLoading: boolean
  isOwner: boolean
  onPress: () => void
}) {
  const control = (
    <Button
      variant="tertiary"
      size="sm"
      isDisabled={isLoading || !isOwner}
      onPress={onPress}
    >
      Buy credits
    </Button>
  )
  if (isOwner) return control
  return (
    <Tooltip delay={250} closeDelay={0}>
      <Tooltip.Trigger className="flex shrink-0">{control}</Tooltip.Trigger>
      <Tooltip.Content placement="left" offset={8} className="text-xs">
        Only the workspace owner can buy credits.
      </Tooltip.Content>
    </Tooltip>
  )
}
