"use client"

import { useMemo, useState } from "react"
import { AnimatePresence, motion, useReducedMotion } from "motion/react"
import { Button, Input, Modal, Spinner, toast } from "@heroui/react"
import { AgentLogo } from "@/app/w/(chat)/_components/chat-agent-logo"
import { agentDisplayName } from "@/app/w/(chat)/_lib/sidebar-data"
import { AgentSelect } from "@/components/agent-select"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"
import { useTeamAgents, type TeamAgent } from "@/lib/api/team-agents"
import { cn } from "@/lib/utils"
import {
  SLACK_CHANNEL_RESOURCE_TYPE,
  slackChannelName,
  slackChannelsForRouting,
  slackConnectionName,
  slackConnectionsForRouting,
  slackRouteSummary,
} from "./_team-external-resource-routes"
import { EmptyProvisioningRow } from "./_provisioning-row"

type TeamConnection = components["schemas"]["teamConnectionResponse"]
type SlackChannel = components["schemas"]["AvailableResource"]
type RouteWizardStep = "connection" | "channel" | "agent"

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

function AddSlackRouteModal({
  isOpen,
  step,
  connections,
  connectionsLoading,
  selectedConnection,
  resources,
  resourcesLoading,
  resourcesError,
  channelQuery,
  onChannelQueryChange,
  agents,
  agentsLoading,
  agentQuery,
  onAgentQueryChange,
  selectedResource,
  selectedAgent,
  isSaving,
  onOpenChange,
  onSelectConnection,
  onSelectChannel,
  onSelectAgent,
  onRetryResources,
  onBack,
  onSave,
}: {
  isOpen: boolean
  step: RouteWizardStep
  connections: TeamConnection[]
  connectionsLoading: boolean
  selectedConnection?: TeamConnection
  resources: SlackChannel[]
  resourcesLoading: boolean
  resourcesError: boolean
  channelQuery: string
  onChannelQueryChange: (query: string) => void
  agents: TeamAgent[]
  agentsLoading: boolean
  agentQuery: string
  onAgentQueryChange: (query: string) => void
  selectedResource?: SlackChannel
  selectedAgent?: TeamAgent
  isSaving: boolean
  onOpenChange: (open: boolean) => void
  onSelectConnection: (connection: TeamConnection) => void
  onSelectChannel: (resource: SlackChannel) => void
  onSelectAgent: (agent: TeamAgent) => void
  onRetryResources: () => void
  onBack: () => void
  onSave: () => void
}) {
  const reduceMotion = useReducedMotion()

  return (
    <Modal isOpen={isOpen} onOpenChange={onOpenChange}>
      <Modal.Backdrop className="bg-background/80 backdrop-blur-sm">
        <Modal.Container placement="center" size="md" className="p-4">
          <Modal.Dialog className="w-full max-w-xl p-6 sm:p-8">
            <Modal.CloseTrigger isDisabled={isSaving} />
            <Modal.Header>
              <Modal.Icon className="size-12 bg-default text-foreground">
                <AppIcon icon="slack" className="h-6 w-6" />
              </Modal.Icon>
              <div className="flex flex-col gap-1">
                <Modal.Heading>Add Slack route</Modal.Heading>
                <p className="text-sm text-muted">
                  Choose a workspace, channel, and agent.
                </p>
              </div>
            </Modal.Header>

            <Modal.Body className="overflow-hidden">
              <div className="min-h-80">
                <AnimatePresence initial={false} mode="wait">
                  <motion.div
                    key={step}
                    initial={reduceMotion ? false : { opacity: 0, x: 12 }}
                    animate={{ opacity: 1, x: 0 }}
                    exit={reduceMotion ? undefined : { opacity: 0, x: -12 }}
                    transition={{
                      duration: reduceMotion ? 0 : 0.18,
                      ease: [0.16, 1, 0.3, 1],
                    }}
                  >
                    {step === "connection" ? (
                      <ConnectionStep
                        connections={connections}
                        isLoading={connectionsLoading}
                        onSelect={onSelectConnection}
                      />
                    ) : null}
                    {step === "channel" ? (
                      <ChannelStep
                        connection={selectedConnection}
                        resources={resources}
                        isLoading={resourcesLoading}
                        isError={resourcesError}
                        query={channelQuery}
                        onQueryChange={onChannelQueryChange}
                        onRetry={onRetryResources}
                        onSelect={onSelectChannel}
                      />
                    ) : null}
                    {step === "agent" ? (
                      <AgentStep
                        agents={agents}
                        isLoading={agentsLoading}
                        query={agentQuery}
                        onQueryChange={onAgentQueryChange}
                        selectedConnection={selectedConnection}
                        selectedAgent={selectedAgent}
                        selectedResource={selectedResource}
                        onSelect={onSelectAgent}
                      />
                    ) : null}
                  </motion.div>
                </AnimatePresence>
              </div>
            </Modal.Body>

            <Modal.Footer>
              <Button
                size="sm"
                variant="tertiary"
                isDisabled={step === "connection" || isSaving}
                onPress={onBack}
              >
                <AppIcon icon="arrow-left" className="h-4 w-4" />
                Back
              </Button>
              <Button
                size="sm"
                variant="primary"
                isDisabled={
                  step !== "agent" ||
                  !selectedAgent ||
                  !selectedResource ||
                  isSaving
                }
                onPress={onSave}
              >
                {isSaving ? <Spinner color="current" size="sm" /> : null}
                Save route
              </Button>
            </Modal.Footer>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}

function ConnectionStep({
  connections,
  isLoading,
  onSelect,
}: {
  connections: TeamConnection[]
  isLoading: boolean
  onSelect: (connection: TeamConnection) => void
}) {
  return (
    <WizardStep
      title="Select a Slack workspace"
      description="Choose from the Slack connections available to this team."
    >
      {isLoading ? (
        <LoadingCards label="Loading Slack workspaces" />
      ) : connections.length ? (
        <CardList>
          {connections.map((connection) => (
            <SelectionCard
              key={connection.id}
              title={slackConnectionName(connection)}
              description="Slack workspace"
              icon={<AppIcon icon="slack" className="h-5 w-5" />}
              onPress={() => onSelect(connection)}
            />
          ))}
        </CardList>
      ) : (
        <EmptyState
          icon="slack"
          title="No Slack workspaces"
          description="Give this team access to a Slack connection before adding a route."
        />
      )}
    </WizardStep>
  )
}

function ChannelStep({
  connection,
  resources,
  isLoading,
  isError,
  query,
  onQueryChange,
  onRetry,
  onSelect,
}: {
  connection?: TeamConnection
  resources: SlackChannel[]
  isLoading: boolean
  isError: boolean
  query: string
  onQueryChange: (query: string) => void
  onRetry: () => void
  onSelect: (resource: SlackChannel) => void
}) {
  return (
    <WizardStep
      title="Select a Slack channel"
      description={`Choose a channel from ${connection ? slackConnectionName(connection) : "this workspace"}.`}
    >
      {isLoading ? (
        <LoadingCards label="Loading Slack channels" />
      ) : isError ? (
        <EmptyState
          icon="circle-alert"
          title="Could not load channels"
          description="Check the Slack connection and try again."
          action={
            <Button size="sm" variant="tertiary" onPress={onRetry}>
              Retry
            </Button>
          }
        />
      ) : (
        <>
          <Input
            aria-label="Search Slack channels"
            placeholder="Search channels"
            value={query}
            onChange={(event) => onQueryChange(event.target.value)}
            className="mb-3 w-full"
          />
          {resources.length ? (
            <CardList>
              {resources.map((resource) => (
                <SelectionCard
                  key={resource.id}
                  title={`#${slackChannelName(resource)}`}
                  icon={<AppIcon icon="hash" className="h-5 w-5 text-muted" />}
                  onPress={() => onSelect(resource)}
                />
              ))}
            </CardList>
          ) : (
            <EmptyState
              icon="hash"
              title={query ? "No matching channels" : "No channels found"}
              description={
                query
                  ? "Try a different channel name."
                  : "This Slack connection did not return any channels."
              }
            />
          )}
        </>
      )}
    </WizardStep>
  )
}

function AgentStep({
  agents,
  isLoading,
  query,
  onQueryChange,
  selectedConnection,
  selectedResource,
  selectedAgent,
  onSelect,
}: {
  agents: TeamAgent[]
  isLoading: boolean
  query: string
  onQueryChange: (query: string) => void
  selectedConnection?: TeamConnection
  selectedResource?: SlackChannel
  selectedAgent?: TeamAgent
  onSelect: (agent: TeamAgent) => void
}) {
  return (
    <WizardStep
      title="Select an agent"
      description="Choose which agent should receive new conversations from this channel."
    >
      {isLoading ? (
        <LoadingCards label="Loading agents" />
      ) : (
        <>
          <Input
            aria-label="Search agents"
            placeholder="Search agents"
            value={query}
            onChange={(event) => onQueryChange(event.target.value)}
            className="mb-3 w-full"
          />
          {agents.length ? (
            <CardList>
              {agents.map((agent) => (
                <SelectionCard
                  key={agent.id}
                  title={agentDisplayName(agent)}
                  description={agent.description?.trim() || undefined}
                  icon={
                    <AgentLogo agent={agent} className="h-8 w-8 rounded-lg" />
                  }
                  selected={agent.id === selectedAgent?.id}
                  showDisclosure={false}
                  onPress={() => onSelect(agent)}
                />
              ))}
            </CardList>
          ) : (
            <EmptyState
              icon="bot"
              title={query ? "No matching agents" : "No agents found"}
              description={
                query
                  ? "Try a different agent name."
                  : "Add an agent to this team before creating a route."
              }
            />
          )}
          {selectedConnection && selectedResource && selectedAgent ? (
            <div
              role="status"
              className="border-primary/20 bg-primary/5 mt-4 flex items-start gap-2 rounded-xl border px-3 py-2.5 text-sm"
            >
              <AppIcon
                icon="info"
                className="text-primary mt-0.5 h-4 w-4 shrink-0"
              />
              <p>
                {slackRouteSummary(
                  slackConnectionName(selectedConnection),
                  slackChannelName(selectedResource),
                  agentDisplayName(selectedAgent)
                )}
              </p>
            </div>
          ) : null}
        </>
      )}
    </WizardStep>
  )
}

function WizardStep({
  title,
  description,
  children,
}: {
  title: string
  description: string
  children: React.ReactNode
}) {
  return (
    <section>
      <h3 className="text-sm font-semibold">{title}</h3>
      <p className="mt-1 text-sm text-muted">{description}</p>
      <div className="mt-4">{children}</div>
    </section>
  )
}

function CardList({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex max-h-64 flex-col gap-2 overflow-y-auto pr-1">
      {children}
    </div>
  )
}

function SelectionCard({
  title,
  description,
  icon,
  selected = false,
  showDisclosure = true,
  onPress,
}: {
  title: string
  description?: string
  icon: React.ReactNode
  selected?: boolean
  showDisclosure?: boolean
  onPress: () => void
}) {
  return (
    <Button
      variant="ghost"
      className={cn(
        "h-auto min-h-16 w-full justify-start rounded-xl border px-4 py-3 text-left transition-[border-color,background-color,transform] duration-150 ease-out motion-reduce:transition-none",
        selected
          ? "border-primary bg-primary/5"
          : "hover:border-primary/40 border-border bg-surface hover:bg-default/60"
      )}
      aria-pressed={showDisclosure ? undefined : selected}
      onPress={onPress}
    >
      <span className="flex w-full min-w-0 items-center gap-3">
        <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-default">
          {icon}
        </span>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm font-medium">{title}</span>
          {description ? (
            <span className="mt-0.5 block truncate text-xs text-muted">
              {description}
            </span>
          ) : null}
        </span>
        {selected ? (
          <span className="bg-primary text-primary-foreground flex size-6 shrink-0 items-center justify-center rounded-full">
            <AppIcon icon="check" className="h-3.5 w-3.5" />
          </span>
        ) : showDisclosure ? (
          <AppIcon
            icon="chevron-right"
            className="h-4 w-4 shrink-0 text-muted"
          />
        ) : null}
      </span>
    </Button>
  )
}

function LoadingCards({ label }: { label: string }) {
  return (
    <div aria-label={label} className="flex flex-col gap-2">
      {[0, 1, 2].map((item) => (
        <div
          key={item}
          className="flex min-h-16 animate-pulse items-center gap-3 rounded-xl border border-border px-4 py-3"
        >
          <span className="size-9 rounded-lg bg-default" />
          <span className="h-3 w-40 rounded-full bg-default" />
        </div>
      ))}
    </div>
  )
}

function EmptyState({
  icon,
  title,
  description,
  action,
}: {
  icon: string
  title: string
  description: string
  action?: React.ReactNode
}) {
  return (
    <div className="flex min-h-52 flex-col items-center justify-center rounded-xl border border-dashed border-border px-6 py-8 text-center">
      <span className="flex size-10 items-center justify-center rounded-xl bg-default text-muted">
        <AppIcon icon={icon} className="h-5 w-5" />
      </span>
      <p className="mt-3 text-sm font-medium">{title}</p>
      <p className="mt-1 max-w-sm text-sm text-muted">{description}</p>
      {action ? <div className="mt-4">{action}</div> : null}
    </div>
  )
}
