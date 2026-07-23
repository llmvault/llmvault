"use client"

import { useQueryClient } from "@tanstack/react-query"
import { toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { queryKeys } from "@/lib/api/query-keys"
import {
  EmptyProvisioningRow,
  ProvisioningRow,
  ProvisioningSkeleton,
  SectionHeader,
} from "@/app/w/settings/teams/[teamId]/_provisioning-row"

export function AgentEnvironmentVariables({ agentId }: { agentId: string }) {
  const queryClient = useQueryClient()
  const variablesQuery = $api.useQuery(
    "get",
    "/v1/agents/{id}/environment-variables",
    { params: { path: { id: agentId } } },
    { enabled: Boolean(agentId) }
  )
  const updateAccess = $api.useMutation(
    "patch",
    "/v1/agents/{id}/environment-variables/{name}"
  )
  const variables = variablesQuery.data?.data ?? []

  function updateVariable(name: string, enabled: boolean) {
    updateAccess.mutate(
      {
        params: { path: { id: agentId, name } },
        body: { enabled },
      },
      {
        onSuccess: () => {
          queryClient.invalidateQueries({
            queryKey: queryKeys.agentEnvironmentVariables(agentId),
          })
          toast.success(
            `${name} ${enabled ? "enabled" : "disabled"} for new sessions`
          )
        },
        onError: (error) =>
          toast.danger(
            extractErrorMessage(
              error,
              "Could not update environment variable access"
            )
          ),
      }
    )
  }

  return (
    <section className="flex flex-col gap-3">
      <SectionHeader
        title="Environment variables"
        description="Variables are inherited from the team by default. Changes apply to new sessions."
      />
      <div className="overflow-hidden rounded-2xl border border-border bg-surface">
        {variablesQuery.isLoading ? (
          <ProvisioningSkeleton />
        ) : variablesQuery.isError ? (
          <EmptyProvisioningRow text="Could not load environment variables." />
        ) : variables.length === 0 ? (
          <EmptyProvisioningRow text="No environment variables are configured for this team yet." />
        ) : (
          variables.map((variable, index) => {
            const name = variable.name ?? ""
            const enabled = variable.enabled !== false
            return (
              <ProvisioningRow
                key={name || index}
                last={index === variables.length - 1}
                icon={
                  <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-default text-muted">
                    <AppIcon icon="braces" className="h-4 w-4" />
                  </span>
                }
                title={name || "Environment variable"}
                subtitle={
                  variable.description?.trim() || "Team environment variable"
                }
                on={enabled}
                disabled={updateAccess.isPending || !name}
                label={`${enabled ? "Disable" : "Enable"} ${name || "environment variable"} for this agent`}
                onChange={(selected) => updateVariable(name, selected)}
              />
            )
          })
        )}
      </div>
    </section>
  )
}
