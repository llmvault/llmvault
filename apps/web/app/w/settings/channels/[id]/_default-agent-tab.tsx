"use client"

import { useQueryClient } from "@tanstack/react-query"
import { Skeleton, toast } from "@heroui/react"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { useTeamAgents } from "@/lib/api/team-agents"
import { AgentSelect } from "@/components/agent-select"
import { FormSection } from "@/app/w/(chat)/automations/_trigger-form-sections"
import type { SidebarAgentResponse } from "@/app/w/(chat)/_lib/sidebar-data"

// The channel's only per-channel agent notion is its default agent: the agent
// that handles mentions and sessions when none is specified. Agents are team
// members now (no per-channel assignment), so the candidates are exactly the
// agents on the channel's own team.
export function DefaultAgentTab({
  channelId,
  teamId,
  defaultAgentId,
}: {
  channelId: string
  teamId: string
  defaultAgentId: string
}) {
  const queryClient = useQueryClient()
  const { agents, isLoading, isError } = useTeamAgents(teamId)
  const updateChannel = $api.useMutation("patch", "/v1/channels/{id}")

  function handleDefaultChange(agentId: string) {
    if (!agentId || agentId === defaultAgentId) return
    updateChannel.mutate(
      {
        params: { path: { id: channelId } },
        body: { default_agent_id: agentId },
      },
      {
        onSuccess: () => {
          toast.success("Default agent updated")
          queryClient.invalidateQueries({
            queryKey: ["get", "/v1/channels/{id}"],
          })
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/channels"] })
        },
        onError: (error) =>
          toast.danger(
            extractErrorMessage(error, "Could not update the default agent")
          ),
      }
    )
  }

  if (isLoading) {
    return (
      <div className="flex flex-col gap-3">
        <Skeleton className="h-16 rounded-2xl" />
      </div>
    )
  }

  if (isError) {
    return (
      <section className="bg-surface rounded-2xl border border-border px-4 py-8 text-center">
        <h2 className="text-sm font-medium text-foreground">
          Couldn&apos;t load agents
        </h2>
        <p className="mt-1 text-sm text-muted-foreground">
          Please try again in a moment.
        </p>
      </section>
    )
  }

  return (
    <FormSection
      title="Default agent"
      description="Handles mentions and sessions in this channel when no agent is specified. Any agent on this channel's team can run here."
    >
      {agents.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          This channel&apos;s team has no agents yet. Install or create an agent
          for the team first.
        </p>
      ) : (
        <AgentSelect
          agents={agents as SidebarAgentResponse[]}
          selectedAgentID={defaultAgentId}
          isLoading={updateChannel.isPending}
          onChange={handleDefaultChange}
          variant="field"
        />
      )}
    </FormSection>
  )
}
