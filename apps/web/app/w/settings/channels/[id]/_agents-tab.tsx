"use client"

import { useMemo, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Skeleton, Spinner, Tooltip, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { AgentSelect } from "@/components/agent-select"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import {
  useAssignChannelAgent,
  useChannelAgents,
  useUnassignChannelAgent,
  type ChannelAgent,
} from "@/lib/api/channel-agents"
import { FormSection } from "@/app/w/(chat)/automations/_trigger-form-sections"
import { AgentLogo } from "@/app/w/(chat)/_components/chat-agent-logo"
import {
  agentDisplayName,
  type SidebarAgentResponse,
} from "@/app/w/(chat)/_lib/sidebar-data"
import { channelAgentsErrorMessage, unassignedAgents } from "./_agents-tab-lib"

export function AgentsTab({
  channelId,
  defaultAgentId,
}: {
  channelId: string
  defaultAgentId: string
}) {
  const queryClient = useQueryClient()

  const assignedQuery = useChannelAgents(channelId)
  const orgAgentsQuery = $api.useQuery("get", "/v1/agents", {
    params: { query: { status: "active", limit: 200 } },
  })
  const assign = useAssignChannelAgent(channelId)
  const unassign = useUnassignChannelAgent(channelId)
  const updateChannel = $api.useMutation("patch", "/v1/channels/{id}")

  const [pendingRemoveId, setPendingRemoveId] = useState<string | null>(null)

  const assigned = useMemo(
    () => assignedQuery.data?.data ?? [],
    [assignedQuery.data?.data]
  )
  const orgAgents = useMemo(
    () => (orgAgentsQuery.data?.data ?? []) as ChannelAgent[],
    [orgAgentsQuery.data?.data]
  )
  const assignable = useMemo(
    () => unassignedAgents(orgAgents, assigned),
    [orgAgents, assigned]
  )

  function refreshChannel() {
    queryClient.invalidateQueries({ queryKey: ["get", "/v1/channels/{id}"] })
    queryClient.invalidateQueries({ queryKey: ["get", "/v1/channels"] })
  }

  function handleAssign(agentId: string) {
    assign.mutate(
      { params: { path: { id: channelId } }, body: { agent_id: agentId } },
      {
        onSuccess: () => {
          toast.success("Agent assigned")
          // The channel's agent set changed — refresh anything that lists it.
          refreshChannel()
        },
        onError: (error) =>
          toast.danger(
            channelAgentsErrorMessage(error, "Could not assign the agent")
          ),
      }
    )
  }

  function handleRemove(agentId: string) {
    setPendingRemoveId(agentId)
    unassign.mutate(
      { params: { path: { id: channelId, agentID: agentId } } },
      {
        onSuccess: () => {
          toast.success("Agent removed")
          refreshChannel()
        },
        onError: (error) =>
          toast.danger(
            channelAgentsErrorMessage(error, "Could not remove the agent")
          ),
        onSettled: () => setPendingRemoveId(null),
      }
    )
  }

  function handleDefaultChange(agentId: string) {
    if (!agentId || agentId === defaultAgentId) return
    updateChannel.mutate(
      { params: { path: { id: channelId } }, body: { default_agent_id: agentId } },
      {
        onSuccess: () => {
          toast.success("Default agent updated")
          refreshChannel()
        },
        onError: (error) =>
          toast.danger(
            extractErrorMessage(error, "Could not update the default agent")
          ),
      }
    )
  }

  if (assignedQuery.isLoading) {
    return (
      <div className="flex flex-col gap-3">
        <Skeleton className="h-16 rounded-2xl" />
        <Skeleton className="h-16 rounded-2xl" />
      </div>
    )
  }

  if (assignedQuery.isError) {
    return (
      <section className="bg-surface rounded-2xl border border-border px-4 py-8 text-center">
        <h2 className="text-sm font-medium text-foreground">
          Couldn&apos;t load assigned agents
        </h2>
        <p className="mt-1 text-sm text-muted-foreground">
          {channelAgentsErrorMessage(
            assignedQuery.error,
            "Please try again in a moment."
          )}
        </p>
      </section>
    )
  }

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-3">
        <div className="flex flex-col gap-1">
          <h2 className="text-sm font-semibold text-foreground">
            Assigned agents
          </h2>
          <p className="text-sm text-muted-foreground">
            Only assigned agents can be mentioned or run sessions in this
            channel. The default is the fallback when no agent is specified.
          </p>
        </div>

        <div className="bg-surface overflow-hidden rounded-2xl border border-border">
          {assigned.length === 0 ? (
            <div className="px-4 py-8 text-center text-sm text-muted-foreground">
              No agents assigned to this channel yet.
            </div>
          ) : (
            assigned.map((agent, index) => (
              <AssignedAgentRow
                key={agent.id ?? index}
                agent={agent}
                isDefault={Boolean(agent.id) && agent.id === defaultAgentId}
                removing={pendingRemoveId === agent.id}
                onRemove={() => agent.id && handleRemove(agent.id)}
                last={index === assigned.length - 1}
              />
            ))
          )}
        </div>
      </div>

      <FormSection
        title="Add an agent"
        description="Give another org agent access to this channel."
      >
        {assignable.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            Every agent is already assigned to this channel.
          </p>
        ) : (
          <div className="flex items-center gap-2">
            <AgentSelect
              agents={assignable as SidebarAgentResponse[]}
              selectedAgentID=""
              isLoading={orgAgentsQuery.isLoading}
              onChange={handleAssign}
              variant="field"
            />
            {assign.isPending ? (
              <Spinner color="current" size="sm" />
            ) : null}
          </div>
        )}
      </FormSection>

      <FormSection
        title="Default agent"
        description="Handles mentions and sessions when no agent is specified. Must be one of the assigned agents."
      >
        <AgentSelect
          agents={assigned as SidebarAgentResponse[]}
          selectedAgentID={defaultAgentId}
          isLoading={updateChannel.isPending}
          onChange={handleDefaultChange}
          variant="field"
        />
      </FormSection>
    </div>
  )
}

function AssignedAgentRow({
  agent,
  isDefault,
  removing,
  onRemove,
  last,
}: {
  agent: ChannelAgent
  isDefault: boolean
  removing: boolean
  onRemove: () => void
  last: boolean
}) {
  const secondary = agent.description?.trim() || agent.model?.trim()
  return (
    <div
      className={`flex items-center gap-3 px-4 py-3.5 ${last ? "" : "border-b border-border"}`}
    >
      <AgentLogo
        agent={agent as SidebarAgentResponse}
        className="h-8 w-8 rounded-lg"
      />
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="truncate text-sm font-medium text-foreground">
          {agentDisplayName(agent as SidebarAgentResponse)}
        </span>
        {secondary ? (
          <span className="truncate text-sm text-muted-foreground">
            {secondary}
          </span>
        ) : null}
      </div>
      {isDefault ? (
        <span className="shrink-0 rounded-md bg-default px-1.5 py-0.5 text-xs font-medium text-muted-foreground">
          Default
        </span>
      ) : null}
      <RemoveButton
        isDefault={isDefault}
        removing={removing}
        onRemove={onRemove}
      />
    </div>
  )
}

function RemoveButton({
  isDefault,
  removing,
  onRemove,
}: {
  isDefault: boolean
  removing: boolean
  onRemove: () => void
}) {
  const button = (
    <Button
      variant="secondary"
      size="sm"
      isIconOnly
      aria-label="Remove agent"
      isDisabled={isDefault || removing}
      onPress={onRemove}
    >
      {removing ? (
        <Spinner color="current" size="sm" />
      ) : (
        <AppIcon icon="trash-2" className="h-4 w-4" />
      )}
    </Button>
  )

  if (!isDefault) return button

  // The default agent can't be removed — the channel always needs a fallback.
  // Wrap the disabled control so the tooltip still explains why.
  return (
    <Tooltip delay={200} closeDelay={0}>
      <Tooltip.Trigger className="flex shrink-0">
        <span>{button}</span>
      </Tooltip.Trigger>
      <Tooltip.Content placement="top" offset={6} className="text-xs">
        This is the channel&apos;s default agent. Change the default first.
      </Tooltip.Content>
    </Tooltip>
  )
}
