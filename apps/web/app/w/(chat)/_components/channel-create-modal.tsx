"use client"

import { useMemo, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { Button, ListBox, Modal, Select, Spinner, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"
import {
  invalidateSessionListQueries,
  type ChannelResponse,
} from "@/app/w/(chat)/_lib/chat-cache"
import { AgentSelect } from "@/components/agent-select"
import { CHANNEL_CATEGORIES, type ChannelCategory } from "@/lib/api/memory"
import { cn } from "@/lib/utils"

type Connection = components["schemas"]["connectionResponse"]
type AvailableResource = components["schemas"]["AvailableResource"]
type Team = components["schemas"]["teamResponse"]

const SLACK_CHANNEL_RESOURCE_TYPE = "slack_channel"

export function ChannelCreateModal({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: (channel: ChannelResponse) => void
}) {
  if (!open) return null

  return (
    <ChannelCreateModalContent
      onCreated={onCreated}
      onOpenChange={onOpenChange}
    />
  )
}

function ChannelCreateModalContent({
  onCreated,
  onOpenChange,
}: {
  onCreated: (channel: ChannelResponse) => void
  onOpenChange: (open: boolean) => void
}) {
  const queryClient = useQueryClient()
  const [selectedConnectionID, setSelectedConnectionID] = useState("")
  const [selectedChannelID, setSelectedChannelID] = useState("")
  const [selectedAgentID, setSelectedAgentID] = useState("")
  const [selectedCategory, setSelectedCategory] = useState<
    ChannelCategory | ""
  >("")
  const [selectedTeamID, setSelectedTeamID] = useState("")
  const [search, setSearch] = useState("")
  const connectionsQuery = $api.useQuery(
    "get",
    "/v1/connections",
    {
      params: { query: { provider: "slack", limit: 100 } },
    },
    { retry: false }
  )
  const connections = useMemo(
    () => (connectionsQuery.data?.data ?? []) as Connection[],
    [connectionsQuery.data?.data]
  )
  const activeConnectionID = selectedConnectionID || connections[0]?.id || ""
  const selectedConnection = connections.find(
    (connection) => connection.id === activeConnectionID
  )
  const resourcesQuery = $api.useQuery(
    "get",
    "/v1/connections/{id}/resources/{type}",
    {
      params: {
        path: {
          id: activeConnectionID,
          type: SLACK_CHANNEL_RESOURCE_TYPE,
        },
      },
    },
    {
      enabled: Boolean(activeConnectionID),
      retry: false,
    }
  )
  const agentsQuery = $api.useQuery(
    "get",
    "/v1/agents",
    { params: { query: { status: "active", limit: 100 } } },
    { retry: false }
  )
  const agents = useMemo(
    () => agentsQuery.data?.data ?? [],
    [agentsQuery.data?.data]
  )
  const defaultAgentID =
    agents.find((agent) => agent.is_default)?.id ?? agents[0]?.id ?? ""
  const activeAgentID = selectedAgentID || defaultAgentID
  // Admin-only endpoint (GET /v1/orgs/current/teams): for non-admin members
  // this returns 403 and `teams` is empty, so the picker is always optional.
  const teamsQuery = $api.useQuery(
    "get",
    "/v1/orgs/current/teams",
    { params: { query: { limit: 100 } } },
    { retry: false }
  )
  const teams = useMemo(
    () => (teamsQuery.data?.data as Team[] | undefined) ?? [],
    [teamsQuery.data?.data]
  )
  const createChannel = $api.useMutation("post", "/v1/channels")
  const resources = useMemo(
    () => (resourcesQuery.data?.resources ?? []) as AvailableResource[],
    [resourcesQuery.data?.resources]
  )
  const filteredResources = useMemo(() => {
    const term = search.trim().toLowerCase()
    if (!term) return resources
    return resources.filter((resource) => {
      const name = resource.name?.toLowerCase() ?? ""
      const id = resource.id?.toLowerCase() ?? ""
      return name.includes(term) || id.includes(term)
    })
  }, [resources, search])
  const selectedResource = resources.find(
    (resource) => resource.id === selectedChannelID
  )

  function selectConnection(id: string) {
    setSelectedConnectionID(id)
    setSelectedChannelID("")
    setSearch("")
  }

  function close(nextOpen: boolean) {
    if (!nextOpen && createChannel.isPending) return
    onOpenChange(nextOpen)
  }

  function handleCreate() {
    if (
      !selectedConnection?.id ||
      !selectedResource?.id ||
      !activeAgentID ||
      !selectedCategory
    )
      return
    // Built as a variable (not an inline literal) so the `category` field —
    // not yet in the generated OpenAPI schema — passes the type check.
    const body = {
      origin: "external",
      external_provider: "slack",
      external_connection_id: selectedConnection.id,
      external_workspace_key: selectedConnection.nango_connection_id,
      external_resource_type: SLACK_CHANNEL_RESOURCE_TYPE,
      external_resource_key: selectedResource.id,
      external_resource_name: selectedResource.name || selectedResource.id,
      default_agent_id: activeAgentID,
      category: selectedCategory,
      team_id: selectedTeamID || undefined,
    }
    createChannel.mutate(
      { body },
      {
        onSuccess: (response) => {
          if (response.channel) {
            invalidateSessionListQueries(queryClient)
            onCreated(response.channel)
          }
          toast.success("Channel created")
          onOpenChange(false)
        },
        onError: (error) =>
          toast.danger(extractErrorMessage(error, "Could not create channel")),
      }
    )
  }

  return (
    <Modal isOpen onOpenChange={close}>
      <Modal.Backdrop className="bg-background/80 backdrop-blur-sm">
        <Modal.Container placement="center" className="p-4">
          <Modal.Dialog className="flex max-h-[calc(100vh-2rem)] w-full max-w-lg flex-col overflow-hidden p-0">
            <Modal.CloseTrigger />
            <div className="flex min-h-0 flex-col">
              <div className="border-b border-border px-6 pt-6 pb-4">
                <div className="flex items-start gap-3 pr-10">
                  <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-default text-foreground">
                    <AppIcon icon="hash" className="h-5 w-5" />
                  </div>
                  <div className="min-w-0">
                    <Modal.Heading>Create channel</Modal.Heading>
                    <p className="mt-1 text-sm text-muted">
                      Select a connected Slack channel.
                    </p>
                  </div>
                </div>
              </div>

              <div className="flex min-h-0 flex-col gap-5 overflow-y-auto px-6 py-5">
                <section className="flex flex-col gap-2">
                  <h3 className="text-xs font-medium tracking-wide text-muted uppercase">
                    Provider
                  </h3>
                  <button
                    type="button"
                    aria-pressed="true"
                    className="border-primary/40 bg-primary/10 flex items-center gap-3 rounded-lg border px-3 py-2 text-left"
                  >
                    <AppIcon
                      icon="message-square"
                      className="h-5 w-5 shrink-0"
                    />
                    <span className="min-w-0 flex-1 text-sm font-medium text-foreground">
                      Slack
                    </span>
                    <AppIcon
                      icon="check"
                      className="text-primary h-4 w-4"
                    />
                  </button>
                </section>

                <section className="flex min-h-0 flex-col gap-2">
                  <div className="flex items-center justify-between gap-3">
                    <h3 className="text-xs font-medium tracking-wide text-muted uppercase">
                      Channel
                    </h3>
                    {resourcesQuery.isFetching ? <Spinner size="sm" /> : null}
                  </div>

                  {connectionsQuery.isLoading ? (
                    <StatusRow label="Loading Slack connections" />
                  ) : connectionsQuery.isError ? (
                    <StatusRow label="Could not load Slack connections" />
                  ) : connections.length === 0 ? (
                    <StatusRow label="No Slack connections" />
                  ) : (
                    <>
                      {connections.length > 1 ? (
                        <div className="flex flex-wrap gap-2">
                          {connections.map((connection) => {
                            const selected =
                              connection.id === activeConnectionID
                            return (
                              <button
                                key={connection.id}
                                type="button"
                                aria-pressed={selected}
                                className={cn(
                                  "rounded-full border px-3 py-1.5 text-sm transition-colors",
                                  selected
                                    ? "border-primary/50 bg-primary/10 text-foreground"
                                    : "border-border text-muted hover:bg-default"
                                )}
                                onClick={() =>
                                  selectConnection(connection.id ?? "")
                                }
                              >
                                {connectionLabel(connection)}
                              </button>
                            )
                          })}
                        </div>
                      ) : null}

                      <div className="bg-field-background flex items-center gap-2 rounded-lg border border-border px-3 py-2">
                        <AppIcon
                          icon="search"
                          className="h-4 w-4 shrink-0 text-muted"
                        />
                        <input
                          type="text"
                          aria-label="Search Slack channels"
                          placeholder="Search channels"
                          value={search}
                          onChange={(event) => setSearch(event.target.value)}
                          className="min-w-0 flex-1 bg-transparent text-sm outline-none placeholder:text-muted"
                        />
                      </div>

                      <div className="max-h-72 min-h-36 overflow-y-auto rounded-lg border border-border">
                        {resourcesQuery.isLoading ? (
                          <StatusRow label="Loading channels" />
                        ) : resourcesQuery.isError ? (
                          <StatusRow label="Could not load channels" />
                        ) : filteredResources.length === 0 ? (
                          <StatusRow label="No channels found" />
                        ) : (
                          <div className="flex flex-col">
                            {filteredResources.map((resource, index) => (
                              <ResourceRow
                                key={resource.id || index}
                                resource={resource}
                                selected={resource.id === selectedChannelID}
                                onSelect={() =>
                                  setSelectedChannelID(resource.id ?? "")
                                }
                              />
                            ))}
                          </div>
                        )}
                      </div>
                    </>
                  )}
                </section>

                <section className="flex flex-col gap-2">
                  <h3 className="text-xs font-medium tracking-wide text-muted uppercase">
                    Category
                  </h3>
                  <p className="text-sm text-muted">
                    Sets what this channel&apos;s agent remembers.
                  </p>
                  <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                    {CHANNEL_CATEGORIES.map((category) => {
                      const selected = category.value === selectedCategory
                      return (
                        <button
                          key={category.value}
                          type="button"
                          aria-pressed={selected}
                          className={cn(
                            "flex flex-col gap-0.5 rounded-lg border px-3 py-2 text-left transition-colors",
                            selected
                              ? "border-primary/40 bg-primary/10"
                              : "border-border hover:bg-default"
                          )}
                          onClick={() => setSelectedCategory(category.value)}
                        >
                          <span className="flex items-center gap-2 text-sm font-medium text-foreground">
                            <span className="min-w-0 flex-1 truncate">
                              {category.label}
                            </span>
                            {selected ? (
                              <AppIcon
                                icon="check"
                                className="text-primary h-4 w-4 shrink-0"
                              />
                            ) : null}
                          </span>
                          <span className="text-xs text-muted">
                            {category.description}
                          </span>
                        </button>
                      )
                    })}
                  </div>
                </section>

                <section className="flex flex-col gap-2">
                  <h3 className="text-xs font-medium tracking-wide text-muted uppercase">
                    Agent
                  </h3>
                  <p className="text-sm text-muted">
                    Mentions in this channel are handled by this agent.
                  </p>
                  <AgentSelect
                    agents={agents}
                    selectedAgentID={activeAgentID}
                    isLoading={agentsQuery.isLoading}
                    onChange={setSelectedAgentID}
                  />
                </section>

                <section className="flex flex-col gap-2">
                  <h3 className="text-xs font-medium tracking-wide text-muted uppercase">
                    Team
                  </h3>
                  <p className="text-sm text-muted">
                    Optionally scope this channel to a team. Leave unset to
                    keep it workspace-wide.
                  </p>
                  <TeamSelect
                    teams={teams}
                    isLoading={teamsQuery.isLoading}
                    selectedTeamID={selectedTeamID}
                    onChange={setSelectedTeamID}
                  />
                </section>
              </div>

              <div className="flex justify-end gap-2 border-t border-border px-6 py-4">
                <Button
                  size="sm"
                  variant="secondary"
                  isDisabled={createChannel.isPending}
                  onPress={() => close(false)}
                >
                  Cancel
                </Button>
                <Button
                  size="sm"
                  variant="primary"
                  isDisabled={
                    !selectedResource?.id ||
                    !activeAgentID ||
                    !selectedCategory ||
                    createChannel.isPending ||
                    resourcesQuery.isLoading
                  }
                  onPress={handleCreate}
                >
                  {createChannel.isPending ? (
                    <Spinner color="current" size="sm" />
                  ) : null}
                  Create
                </Button>
              </div>
            </div>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}

function ResourceRow({
  resource,
  selected,
  onSelect,
}: {
  resource: AvailableResource
  selected: boolean
  onSelect: () => void
}) {
  const name = resource.name || resource.id || "Channel"
  return (
    <button
      type="button"
      aria-pressed={selected}
      className={cn(
        "flex items-center gap-3 border-b border-border px-3 py-2.5 text-left transition-colors last:border-b-0",
        selected ? "bg-primary/10" : "hover:bg-default"
      )}
      onClick={onSelect}
    >
      <AppIcon icon="hash" className="h-4 w-4 shrink-0 text-muted" />
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium text-foreground">{name}</p>
      </div>
      {selected ? (
        <AppIcon icon="check" className="text-primary h-4 w-4 shrink-0" />
      ) : null}
    </button>
  )
}

function TeamSelect({
  teams,
  isLoading,
  selectedTeamID,
  onChange,
}: {
  teams: Team[]
  isLoading: boolean
  selectedTeamID: string
  onChange: (teamID: string) => void
}) {
  const NO_TEAM_KEY = ""
  const selected = teams.find((team) => team.id === selectedTeamID)
  const selectedLabel = isLoading
    ? "Loading teams"
    : selected?.name || "No team"

  return (
    <Select
      aria-label="Team"
      selectedKey={selectedTeamID || NO_TEAM_KEY}
      onSelectionChange={(key) => {
        if (key !== null) onChange(String(key))
      }}
      isDisabled={isLoading}
      className="w-full"
    >
      <Select.Trigger className="h-9 w-full justify-between px-3 text-sm transition-colors">
        <span className="truncate">{selectedLabel}</span>
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover className="p-1.5">
        <ListBox>
          <ListBox.Item id={NO_TEAM_KEY} textValue="No team">
            <span className="text-sm font-medium">No team</span>
          </ListBox.Item>
          {teams.map((team) => (
            <ListBox.Item
              key={team.id}
              id={team.id ?? ""}
              textValue={team.name || team.id || "Team"}
            >
              <span className="text-sm font-medium">
                {team.name || "Untitled team"}
              </span>
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}

function StatusRow({ label }: { label: string }) {
  return (
    <div className="flex min-h-24 items-center justify-center px-3 py-4 text-sm text-muted">
      {label}
    </div>
  )
}

function connectionLabel(connection: Connection) {
  return (
    connection.display_name ||
    connection.nango_connection_id ||
    connection.id ||
    "Slack"
  )
}
