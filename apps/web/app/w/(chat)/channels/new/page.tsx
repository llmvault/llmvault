"use client"

import { FormEvent, useEffect, useMemo, useState } from "react"
import { useRouter, useSearchParams } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Input, ListBox, Select, Spinner, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { cn } from "@/lib/utils"
import { AgentSelect } from "@/components/agent-select"
import { CHANNEL_CATEGORIES, type ChannelCategory } from "@/lib/api/memory"
import { resolveScopedAgentID, useTeamAgents } from "@/lib/api/team-agents"
import { channelRouteSlug } from "@/app/w/(chat)/_lib/sidebar-data"
import { invalidateSessionListQueries } from "@/app/w/(chat)/_lib/chat-cache"
import {
  FormSection,
  InlineNotice,
} from "@/app/w/(chat)/automations/_trigger-form-sections"
import {
  CHANNEL_PROVIDERS,
  connectionsForProvider,
  ProviderCards,
  ResourceRow,
  StatusRow,
  TeamSelect,
  LockedTeamRow,
  connectionLabel,
  SLACK_CHANNEL_RESOURCE_TYPE,
  type ChannelProviderMeta,
  type ChannelProviderStatus,
} from "./_form"

export default function NewChannelPage() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const queryClient = useQueryClient()

  const teamParam = searchParams.get("team") ?? ""
  const teamLocked = Boolean(teamParam)

  const [provider, setProvider] = useState("")
  const [selectedConnectionID, setSelectedConnectionID] = useState("")
  const [selectedResourceID, setSelectedResourceID] = useState("")
  const [name, setName] = useState("")
  const [nameEdited, setNameEdited] = useState(false)
  const [search, setSearch] = useState("")
  const [agentID, setAgentID] = useState("")
  const [category, setCategory] = useState<ChannelCategory | "">("")
  const [selectedTeamID, setSelectedTeamID] = useState(teamParam)

  useEffect(() => {
    setSelectedTeamID(teamParam)
  }, [teamParam])

  const connectionsQuery = $api.useQuery(
    "get",
    "/v1/connections",
    { params: { query: { limit: 100 } } },
    { retry: false }
  )
  const connections = useMemo(
    () => connectionsQuery.data?.data ?? [],
    [connectionsQuery.data?.data]
  )

  const teamConnectionsQuery = $api.useQuery(
    "get",
    "/v1/orgs/current/teams/{teamID}/connections",
    { params: { path: { teamID: selectedTeamID } } },
    { enabled: Boolean(selectedTeamID), retry: false }
  )
  const teamConnectionIDs = useMemo(() => {
    const set = new Set<string>()
    for (const connection of teamConnectionsQuery.data?.data ?? []) {
      if (connection.id) set.add(connection.id)
    }
    return set
  }, [teamConnectionsQuery.data?.data])

  function providerStatus(meta: ChannelProviderMeta): ChannelProviderStatus {
    if (!selectedTeamID) {
      return {
        available: false,
        hint: "Select a team first to link an external channel.",
        showConnect: false,
      }
    }
    if (teamConnectionsQuery.isLoading) {
      return {
        available: false,
        hint: "Checking team connections…",
        showConnect: false,
      }
    }
    const granted = connectionsForProvider(meta, connections).filter(
      (connection) => connection.id && teamConnectionIDs.has(connection.id)
    )
    if (granted.length === 0) {
      return {
        available: false,
        hint: `Connect ${meta.label} in settings to link a channel.`,
        showConnect: true,
      }
    }
    return { available: true, hint: meta.description, showConnect: false }
  }

  const meta = provider
    ? (CHANNEL_PROVIDERS.find((p) => p.provider === provider) ?? null)
    : null
  const selectedProviderAvailable = meta ? providerStatus(meta).available : true

  useEffect(() => {
    if (meta && !selectedProviderAvailable) {
      setProvider("")
      setSelectedConnectionID("")
      setSelectedResourceID("")
      setSearch("")
    }
  }, [meta, selectedProviderAvailable])

  const providerConnections = useMemo(
    () =>
      connectionsForProvider(meta, connections).filter(
        (connection) => connection.id && teamConnectionIDs.has(connection.id)
      ),
    [meta, connections, teamConnectionIDs]
  )
  const activeConnectionID =
    selectedConnectionID || providerConnections[0]?.id || ""
  const selectedConnection = providerConnections.find(
    (connection) => connection.id === activeConnectionID
  )

  const resourcesQuery = $api.useQuery(
    "get",
    "/v1/connections/{id}/resources/{type}",
    {
      params: {
        path: { id: activeConnectionID, type: SLACK_CHANNEL_RESOURCE_TYPE },
      },
    },
    { enabled: Boolean(activeConnectionID), retry: false }
  )
  const resources = useMemo(
    () => resourcesQuery.data?.resources ?? [],
    [resourcesQuery.data?.resources]
  )
  const filteredResources = useMemo(() => {
    const term = search.trim().toLowerCase()
    if (!term) return resources
    return resources.filter((resource) => {
      const resourceName = resource.name?.toLowerCase() ?? ""
      const resourceID = resource.id?.toLowerCase() ?? ""
      return resourceName.includes(term) || resourceID.includes(term)
    })
  }, [resources, search])
  const selectedResource = resources.find(
    (resource) => resource.id === selectedResourceID
  )

  useEffect(() => {
    if (selectedResource && !nameEdited) {
      setName(selectedResource.name || selectedResource.id || "")
    }
  }, [selectedResource, nameEdited])

  const teamsQuery = $api.useQuery(
    "get",
    "/v1/orgs/current/teams",
    { params: { query: { limit: 100 } } },
    { retry: false }
  )
  const teams = useMemo(
    () => teamsQuery.data?.data ?? [],
    [teamsQuery.data?.data]
  )

  const { agents, isLoading: agentsLoading } = useTeamAgents(selectedTeamID)
  const activeAgentID = resolveScopedAgentID(agents, agentID, undefined)

  const createChannel = $api.useMutation("post", "/v1/channels")
  const saving = createChannel.isPending

  function selectProvider(next: ChannelProviderMeta) {
    setProvider(next.provider)
    setSelectedConnectionID("")
    setSelectedResourceID("")
    setSearch("")
  }

  function selectConnection(id: string) {
    setSelectedConnectionID(id)
    setSelectedResourceID("")
    setSearch("")
  }

  const isExternal = Boolean(
    meta && selectedConnection?.id && selectedResource?.id
  )
  const trimmedName = name.trim()
  const canSubmit = Boolean(
    !saving && trimmedName && selectedTeamID && category && activeAgentID
  )

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!trimmedName) {
      toast.danger("Name is required")
      return
    }
    if (!selectedTeamID) {
      toast.danger("Select a team")
      return
    }
    if (!category) {
      toast.danger("Select a category")
      return
    }
    if (!activeAgentID) {
      toast.danger("Select an agent")
      return
    }

    const body =
      isExternal && selectedConnection && selectedResource
        ? {
            origin: "external",
            external_provider: provider,
            external_connection_id: selectedConnection.id,
            external_workspace_key: selectedConnection.nango_connection_id,
            external_resource_type: SLACK_CHANNEL_RESOURCE_TYPE,
            external_resource_key: selectedResource.id,
            external_resource_name: trimmedName,
            name: trimmedName,
            default_agent_id: activeAgentID,
            category,
            team_id: selectedTeamID,
          }
        : {
            name: trimmedName,
            default_agent_id: activeAgentID,
            category,
            team_id: selectedTeamID,
          }

    createChannel.mutate(
      { body },
      {
        onSuccess: (response) => {
          invalidateSessionListQueries(queryClient)
          toast.success("Channel created")
          if (response.channel) {
            router.push(`/w/channels/${channelRouteSlug(response.channel)}`)
          } else {
            router.back()
          }
        },
        onError: (error) =>
          toast.danger(extractErrorMessage(error, "Could not create channel")),
      }
    )
  }

  return (
    <div className="h-full overflow-y-auto bg-background text-foreground">
      <div className="mx-auto w-full max-w-2xl px-6 py-12">
        <div className="flex flex-col gap-8">
          <button
            type="button"
            onClick={() => router.back()}
            className="text-muted-foreground flex w-fit items-center gap-1.5 text-sm transition-colors hover:text-foreground"
          >
            <AppIcon icon="arrow-left" className="h-4 w-4" />
            Back
          </button>

          <header className="flex items-center gap-3">
            <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl bg-default text-foreground">
              <AppIcon icon="hash" className="h-6 w-6" />
            </div>
            <div>
              <h1 className="text-lg font-semibold text-foreground">
                Create channel
              </h1>
              <p className="text-muted-foreground mt-1 text-sm">
                Create a channel for a team. You can optionally link it to
                Slack.
              </p>
            </div>
          </header>

          <form onSubmit={handleSubmit} className="flex flex-col gap-6">
            <FormSection
              title="Name"
              description="Shown in your sidebar. This is the channel's name."
            >
              <Input
                value={name}
                onChange={(event) => {
                  setNameEdited(true)
                  setName(event.target.value)
                }}
                placeholder="Name this channel"
                className="h-9 w-full rounded-md"
              />
            </FormSection>

            <FormSection
              title="Team"
              description="The team this channel belongs to."
            >
              {teamLocked ? (
                <LockedTeamRow
                  label={
                    teamsQuery.isLoading
                      ? "Loading team"
                      : teams.find((team) => team.id === selectedTeamID)
                          ?.name || "Selected team"
                  }
                />
              ) : (
                <TeamSelect
                  teams={teams}
                  isLoading={teamsQuery.isLoading}
                  selectedTeamID={selectedTeamID}
                  onChange={(id) => {
                    setSelectedTeamID(id)
                    setAgentID("")
                  }}
                />
              )}
            </FormSection>

            <FormSection
              title="Category"
              description={
                CHANNEL_CATEGORIES.find((option) => option.value === category)
                  ?.description ?? "Sets what this channel's agent remembers."
              }
            >
              <Select
                aria-label="Category"
                selectedKey={category || null}
                onSelectionChange={(key) =>
                  setCategory(
                    key === null ? "" : (String(key) as ChannelCategory)
                  )
                }
                className="w-full"
              >
                <Select.Trigger className="h-9 w-full justify-between px-3 text-sm">
                  <span className="truncate">
                    {CHANNEL_CATEGORIES.find(
                      (option) => option.value === category
                    )?.label ?? "Select category"}
                  </span>
                  <Select.Indicator />
                </Select.Trigger>
                <Select.Popover className="p-1.5">
                  <ListBox>
                    {CHANNEL_CATEGORIES.map((option) => (
                      <ListBox.Item
                        key={option.value}
                        id={option.value}
                        textValue={option.label}
                      >
                        {option.label}
                      </ListBox.Item>
                    ))}
                  </ListBox>
                </Select.Popover>
              </Select>
            </FormSection>

            <FormSection
              title="Agent"
              description="Mentions in this channel are handled by this agent."
            >
              {!selectedTeamID ? (
                <InlineNotice
                  icon="bot"
                  title="Select a team first"
                  body="Agents are scoped to the team that owns this channel."
                />
              ) : agents.length === 0 && !agentsLoading ? (
                <InlineNotice
                  icon="bot"
                  title="No agents on this team"
                  body="Add an agent to this team before creating a channel."
                />
              ) : (
                <AgentSelect
                  agents={agents}
                  selectedAgentID={activeAgentID}
                  isLoading={agentsLoading}
                  onChange={setAgentID}
                  variant="field"
                />
              )}
            </FormSection>

            <FormSection
              title="Connect an external channel (optional)"
              description="Optionally mirror this to a Slack channel. You can also connect one later by editing the channel."
            >
              <div className="flex flex-col gap-3">
                <ProviderCards
                  value={provider}
                  onChange={selectProvider}
                  statusFor={providerStatus}
                />

                {meta ? (
                  connectionsQuery.isLoading ? (
                    <div className="h-9 animate-pulse rounded-md bg-default" />
                  ) : providerConnections.length === 0 ? (
                    <InlineNotice
                      icon="message-square"
                      title={`No ${meta.label} connections`}
                      body={`Connect ${meta.label} in settings before linking a channel.`}
                    />
                  ) : (
                    <div className="flex flex-col gap-2">
                      {providerConnections.length > 1 ? (
                        <div className="flex flex-wrap gap-2">
                          {providerConnections.map((connection) => {
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
                          aria-label={`Search ${meta.label} channels`}
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
                                selected={resource.id === selectedResourceID}
                                onSelect={() =>
                                  setSelectedResourceID(resource.id ?? "")
                                }
                              />
                            ))}
                          </div>
                        )}
                      </div>
                    </div>
                  )
                ) : null}
              </div>
            </FormSection>

            <div className="flex justify-end">
              <Button
                type="submit"
                variant="primary"
                size="sm"
                className="shrink-0"
                isDisabled={!canSubmit}
              >
                {saving ? (
                  <Spinner color="current" size="sm" />
                ) : (
                  <AppIcon icon="plus" className="h-4 w-4" />
                )}
                Create channel
              </Button>
            </div>
          </form>
        </div>
      </div>
    </div>
  )
}
