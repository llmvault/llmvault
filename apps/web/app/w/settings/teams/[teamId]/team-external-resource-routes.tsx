"use client"

import { useMemo, useState } from "react"
import { Button, toast } from "@heroui/react"
import { agentDisplayName } from "@/app/w/(chat)/_lib/sidebar-data"
import { AgentSelect } from "@/components/agent-select"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"
import { useTeamAgents } from "@/lib/api/team-agents"
import {
  SLACK_CHANNEL_RESOURCE_TYPE,
  slackChannelName,
  slackChannelsForRouting,
  slackConnectionsForRouting,
} from "./_team-external-resource-routes"
import { EmptyProvisioningRow } from "./_provisioning-row"
import {
  AddSlackRouteModal,
  type RouteWizardStep,
} from "./team-external-resource-route-modal"

type TeamConnection = components["schemas"]["teamConnectionResponse"]
type SlackChannel = components["schemas"]["AvailableResource"]

export function TeamExternalResourceRoutes({ teamId }: { teamId: string }) {
  const routesQuery = $api.useQuery(
    "get",
    "/v1/orgs/current/teams/{id}/external-resource-routes",
    { params: { path: { id: teamId } } }
  )
  const connectionsQuery = $api.useQuery(
    "get",
    "/v1/orgs/current/teams/{teamID}/connections",
    { params: { path: { teamID: teamId } } }
  )
  const { agents, isLoading: agentsLoading } = useTeamAgents(teamId)
  const createRoute = $api.useMutation(
    "post",
    "/v1/orgs/current/teams/{id}/external-resource-routes"
  )
  const deleteRoute = $api.useMutation(
    "delete",
    "/v1/orgs/current/teams/{id}/external-resource-routes/{routeID}"
  )
  const updateRoute = $api.useMutation(
    "patch",
    "/v1/orgs/current/teams/{id}/external-resource-routes/{routeID}"
  )

  const [modalOpen, setModalOpen] = useState(false)
  const [step, setStep] = useState<RouteWizardStep>("connection")
  const [connectionID, setConnectionID] = useState("")
  const [resourceKey, setResourceKey] = useState("")
  const [agentID, setAgentID] = useState("")
  const [channelQuery, setChannelQuery] = useState("")
  const [agentQuery, setAgentQuery] = useState("")
  const [deleteTarget, setDeleteTarget] = useState<{
    id: string
    channelName: string
  } | null>(null)

  const connections = useMemo(
    () => slackConnectionsForRouting(connectionsQuery.data?.data ?? []),
    [connectionsQuery.data?.data]
  )
  const resourcesQuery = $api.useQuery(
    "get",
    "/v1/connections/{id}/resources/{type}",
    {
      params: {
        path: {
          id: connectionID,
          type: SLACK_CHANNEL_RESOURCE_TYPE,
        },
      },
    },
    { enabled: modalOpen && Boolean(connectionID), retry: false }
  )
  const resources = useMemo(
    () => slackChannelsForRouting(resourcesQuery.data?.resources ?? []),
    [resourcesQuery.data?.resources]
  )
  const selectedConnection = connections.find(
    (connection) => connection.id === connectionID
  )
  const selectedResource = resources.find((item) => item.id === resourceKey)
  const selectedAgent = agents.find((agent) => agent.id === agentID)
  const filteredResources = useMemo(() => {
    const query = channelQuery.trim().toLowerCase()
    if (!query) return resources
    return resources.filter((resource) =>
      slackChannelName(resource).toLowerCase().includes(query)
    )
  }, [channelQuery, resources])
  const filteredAgents = useMemo(() => {
    const query = agentQuery.trim().toLowerCase()
    if (!query) return agents
    return agents.filter((agent) =>
      agentDisplayName(agent).toLowerCase().includes(query)
    )
  }, [agentQuery, agents])
  const ready = Boolean(
    connectionID && resourceKey && selectedResource && selectedAgent
  )
  const routes = useMemo(
    () =>
      (routesQuery.data?.data ?? []).filter(
        (route) => route.resource_type === SLACK_CHANNEL_RESOURCE_TYPE
      ),
    [routesQuery.data?.data]
  )

  function refresh() {
    void routesQuery.refetch()
  }

  function resetWizard() {
    setStep("connection")
    setConnectionID("")
    setResourceKey("")
    setAgentID("")
    setChannelQuery("")
    setAgentQuery("")
  }

  function closeModal() {
    if (createRoute.isPending) return
    setModalOpen(false)
    resetWizard()
  }

  function selectConnection(connection: TeamConnection) {
    if (!connection.id) return
    setConnectionID(connection.id)
    setResourceKey("")
    setAgentID("")
    setChannelQuery("")
    setStep("channel")
  }

  function selectChannel(channel: SlackChannel) {
    if (!channel.id) return
    setResourceKey(channel.id)
    setAgentID("")
    setAgentQuery("")
    setStep("agent")
  }

  function goBack() {
    if (step === "agent") {
      setStep("channel")
      return
    }
    if (step === "channel") setStep("connection")
  }

  function create() {
    if (!ready || !selectedResource || !selectedAgent?.id) return
    createRoute.mutate(
      {
        params: { path: { id: teamId } },
        body: {
          connection_id: connectionID,
          agent_id: selectedAgent.id,
          resource_type: SLACK_CHANNEL_RESOURCE_TYPE,
          resource_key: resourceKey,
          resource_name: slackChannelName(selectedResource),
        },
      },
      {
        onSuccess: () => {
          toast.success("Slack channel route created")
          refresh()
          setModalOpen(false)
          resetWizard()
        },
        onError: (error) =>
          toast.danger(
            extractErrorMessage(error, "Could not create Slack channel route")
          ),
      }
    )
  }

  function remove(routeID: string) {
    deleteRoute.mutate(
      { params: { path: { id: teamId, routeID } } },
      {
        onSuccess: () => {
          toast.success("Slack channel route removed")
          refresh()
          setDeleteTarget(null)
        },
        onError: (error) =>
          toast.danger(
            extractErrorMessage(error, "Could not remove Slack channel route")
          ),
      }
    )
  }

  function assignAgent(routeID: string, nextAgentID: string) {
    if (!nextAgentID || updateRoute.isPending) return
    updateRoute.mutate(
      {
        params: { path: { id: teamId, routeID } },
        body: { agent_id: nextAgentID },
      },
      {
        onSuccess: () => {
          toast.success("Slack channel route updated")
          refresh()
        },
        onError: (error) =>
          toast.danger(
            extractErrorMessage(error, "Could not update Slack channel route")
          ),
      }
    )
  }

  return (
    <section className="flex flex-col gap-3">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h2 className="text-sm font-medium">Slack routing</h2>
          <p className="mt-1 max-w-3xl text-sm text-muted">
            Send new conversations from a Slack channel to one agent in this
            team. Existing conversation affinity always takes precedence.
          </p>
        </div>
        <Button
          size="sm"
          variant="primary"
          className="shrink-0 self-start"
          onPress={() => {
            resetWizard()
            setModalOpen(true)
          }}
        >
          <AppIcon icon="plus" className="h-4 w-4" />
          Add route
        </Button>
      </div>

      <div className="overflow-hidden rounded-2xl border border-border bg-surface">
        {routesQuery.isLoading ? (
          <div className="p-4 text-sm text-muted">Loading routes…</div>
        ) : routes.length ? (
          routes.map((route, index) => {
            const connection = connections.find(
              (item) => item.id === route.connection_id
            )
            return (
              <div
                key={route.id}
                className={`flex flex-col gap-3 px-4 py-3 sm:flex-row sm:items-center ${
                  index < routes.length - 1 ? "border-b border-border" : ""
                }`}
              >
                <AppIcon
                  icon="slack"
                  className="hidden h-4 w-4 shrink-0 text-muted sm:block"
                />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">
                    #{route.resource_name || route.resource_key}
                  </p>
                  <p className="truncate text-xs text-muted">
                    {connection?.name || "Slack connection"}
                  </p>
                </div>
                <div className="min-w-0">
                  <AgentSelect
                    agents={agents}
                    selectedAgentID={route.agent_id ?? ""}
                    isLoading={agentsLoading}
                    onChange={(nextAgentID) => {
                      if (route.id && nextAgentID !== route.agent_id) {
                        assignAgent(route.id, nextAgentID)
                      }
                    }}
                  />
                </div>
                <Button
                  size="sm"
                  variant="ghost"
                  isIconOnly
                  aria-label={`Remove route for ${route.resource_name || route.resource_key}`}
                  isDisabled={deleteRoute.isPending || !route.id}
                  onPress={() => {
                    if (!route.id) return
                    setDeleteTarget({
                      id: route.id,
                      channelName:
                        route.resource_name || route.resource_key || "channel",
                    })
                  }}
                >
                  <AppIcon icon="trash-2" className="h-4 w-4" />
                </Button>
              </div>
            )
          })
        ) : (
          <EmptyProvisioningRow text="No Slack channels are routed to this team yet." />
        )}
      </div>

      <AddSlackRouteModal
        isOpen={modalOpen}
        step={step}
        connections={connections}
        connectionsLoading={connectionsQuery.isLoading}
        selectedConnection={selectedConnection}
        resources={filteredResources}
        resourcesLoading={resourcesQuery.isLoading}
        resourcesError={resourcesQuery.isError}
        channelQuery={channelQuery}
        onChannelQueryChange={setChannelQuery}
        agents={filteredAgents}
        agentsLoading={agentsLoading}
        agentQuery={agentQuery}
        onAgentQueryChange={setAgentQuery}
        selectedResource={selectedResource}
        selectedAgent={selectedAgent}
        isSaving={createRoute.isPending}
        onOpenChange={(open) => {
          if (!open) closeModal()
        }}
        onSelectConnection={selectConnection}
        onSelectChannel={selectChannel}
        onSelectAgent={(agent) => agent.id && setAgentID(agent.id)}
        onRetryResources={() => void resourcesQuery.refetch()}
        onBack={goBack}
        onSave={create}
      />

      <ConfirmDialog
        open={deleteTarget !== null}
        pending={deleteRoute.isPending}
        heading="Remove Slack route?"
        description={
          deleteTarget
            ? `New @hivy pings in #${deleteTarget.channelName.replace(/^#/, "")} will no longer be routed to this team. Existing conversations are unaffected.`
            : ""
        }
        confirmLabel="Remove route"
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null)
        }}
        onConfirm={() => {
          if (deleteTarget) remove(deleteTarget.id)
        }}
      />
    </section>
  )
}
