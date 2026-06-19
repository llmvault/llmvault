"use client"

import { Spinner } from "@heroui/react"
import { $api } from "@/lib/api/hooks"
import { useAuth } from "@/lib/auth/auth-context"

export function WorkspaceProvisioningGate({
  children,
}: {
  children: React.ReactNode
}) {
  const { activeOrg, isLoading: authLoading } = useAuth()

  const orgQuery = $api.useQuery(
    "get",
    "/v1/orgs/current",
    {},
    {
      enabled: Boolean(activeOrg?.id),
      retry: false,
    }
  )

  const agentsQuery = $api.useQuery(
    "get",
    "/v1/agents",
    {
      params: {
        query: {
          limit: 50,
        },
      },
    },
    {
      enabled: Boolean(activeOrg?.id),
      retry: false,
    }
  )

  if (!authLoading && !orgQuery.isLoading && !agentsQuery.isLoading) {
    return children
  }

  return (
    <div
      className="bg-surface flex h-screen w-screen items-center justify-center text-foreground"
      role="status"
      aria-label="Loading workspace"
    >
      <Spinner size="lg" />
    </div>
  )
}
