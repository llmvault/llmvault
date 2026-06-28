"use client"

import { FormEvent, useEffect, useMemo, useState } from "react"
import { useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { Button, ListBox, Select, Spinner, toast } from "@heroui/react"
import { Icon } from "@iconify/react"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"
import {
  automationSourceLabel,
  automationTriggerDefaultInstructions,
  automationTriggerDefaultValue,
  automationTriggerKey,
  type AutomationItem,
  type InstalledTrigger,
} from "@/app/w/(chat)/automations/_data"
import { AgentSelect } from "@/app/w/(chat)/automations/_agent-select"
import {
  defaultEmojiGlyph,
  normalizeEmojiName,
  SlackEmojiPicker,
} from "@/app/w/(chat)/automations/_slack-emoji-picker"
import {
  FieldSkeleton,
  FormSection,
  InlineNotice,
} from "@/app/w/(chat)/automations/_trigger-form-sections"

type Connection = components["schemas"]["connectionResponse"]
type AvailableResource = components["schemas"]["AvailableResource"]
const slackReactionKey = "reaction_added"
const slackChannelResourceType = "slack_channel"
export function TriggerInstallForm({
  automation,
  trigger,
}: {
  automation: AutomationItem
  trigger?: InstalledTrigger
}) {
  const triggerKey = trigger?.trigger_key || automationTriggerKey(automation)

  if (automation.provider === "slack" && triggerKey === slackReactionKey) {
    return (
      <SlackReactionInstallForm automation={automation} trigger={trigger} />
    )
  }

  return (
    <section className="flex flex-col gap-3">
      <div>
        <h2 className="text-sm font-semibold text-foreground">
          Trigger setup unavailable
        </h2>
        <p className="text-muted-foreground mt-1 text-sm leading-5">
          This trigger template is not supported by the installer yet.
        </p>
      </div>
    </section>
  )
}

function SlackReactionInstallForm({
  automation,
  trigger,
}: {
  automation: AutomationItem
  trigger?: InstalledTrigger
}) {
  const router = useRouter()
  const queryClient = useQueryClient()
  const triggerID = trigger?.id || ""
  const connectionsQuery = $api.useQuery(
    "get",
    "/v1/connections",
    {
      params: { query: { provider: "slack", limit: 100 } },
    },
    { retry: false }
  )
  const agentsQuery = $api.useQuery("get", "/v1/agents", {
    params: { query: { status: "active", limit: 100 } },
  })
  const createTrigger = $api.useMutation("post", "/v1/triggers")
  const updateTrigger = $api.useMutation("patch", "/v1/triggers/{id}")
  const defaultEmojiName = normalizeEmojiName(
    trigger?.trigger_value ||
      automationTriggerDefaultValue(automation) ||
      "eyes"
  )
  const [connectionID, setConnectionID] = useState(trigger?.connection_id || "")
  const [resourceID, setResourceID] = useState(
    trigger?.external_resource_key || ""
  )
  const [agentID, setAgentID] = useState(trigger?.agent_id || "")
  const [emojiName, setEmojiName] = useState(defaultEmojiName)
  const [emojiGlyph, setEmojiGlyph] = useState(
    defaultEmojiGlyph(defaultEmojiName)
  )
  const [instructions, setInstructions] = useState(
    trigger?.instructions || automationTriggerDefaultInstructions(automation)
  )

  const connections = useMemo(
    () => (connectionsQuery.data?.data ?? []) as Connection[],
    [connectionsQuery.data?.data]
  )
  const activeConnectionID = connectionID || connections[0]?.id || ""
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
          type: slackChannelResourceType,
        },
      },
    },
    {
      enabled: Boolean(activeConnectionID),
      retry: false,
    }
  )
  const initialResource = useMemo(() => {
    if (!trigger?.external_resource_key) return null
    return {
      id: trigger.external_resource_key,
      name: trigger.external_resource_name || trigger.external_resource_key,
      type: slackChannelResourceType,
    } satisfies AvailableResource
  }, [trigger?.external_resource_key, trigger?.external_resource_name])
  const resources = useMemo(() => {
    const list = (
      (resourcesQuery.data?.resources ?? []) as AvailableResource[]
    ).filter((resource) => Boolean(resource.id))
    if (
      initialResource?.id &&
      !list.some((resource) => resource.id === initialResource.id)
    ) {
      return [initialResource, ...list]
    }
    return list
  }, [initialResource, resourcesQuery.data?.resources])
  const selectedResource = useMemo(
    () => resources.find((resource) => resource.id === resourceID),
    [resourceID, resources]
  )
  const agents = useMemo(
    () => (agentsQuery.data?.data ?? []).filter((agent) => agent.id),
    [agentsQuery.data?.data]
  )
  const selectedAgent = useMemo(
    () => agents.find((agent) => agent.id === agentID),
    [agentID, agents]
  )
  const existingTrigger = useMemo(
    () =>
      selectedAgent?.triggers?.some(
        (item) =>
          item.id !== triggerID &&
          item.provider === "slack" &&
          item.connection_id === activeConnectionID &&
          item.trigger_key === slackReactionKey &&
          normalizeEmojiName(item.trigger_value ?? "") === emojiName &&
          item.source_slug ===
            triggerSourceSlug(
              "slack",
              slackReactionKey,
              selectedResource?.id ?? "",
              emojiName
            )
      ) ?? false,
    [
      activeConnectionID,
      emojiName,
      selectedAgent,
      selectedResource?.id,
      triggerID,
    ]
  )

  useEffect(() => {
    if (!connectionID && connections[0]?.id) {
      setConnectionID(connections[0].id)
    }
  }, [connectionID, connections])

  useEffect(() => {
    if (!resourceID && resources[0]?.id) {
      setResourceID(resources[0].id)
    }
  }, [resourceID, resources])

  useEffect(() => {
    if (agents.length === 0) {
      if (agentID) setAgentID("")
      return
    }
    const currentAllowed = agents.some((agent) => agent.id === agentID)
    if (currentAllowed) return
    setAgentID(agents[0]?.id ?? "")
  }, [agentID, agents])

  const isLoading =
    connectionsQuery.isLoading ||
    resourcesQuery.isLoading ||
    agentsQuery.isLoading
  const canSubmit =
    !isLoading &&
    !createTrigger.isPending &&
    !updateTrigger.isPending &&
    !existingTrigger &&
    Boolean(
      activeConnectionID &&
      selectedResource?.id &&
      agentID &&
      emojiName &&
      instructions.trim()
    )

  function handleConnectionChange(id: string) {
    setConnectionID(id)
    setResourceID("")
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!selectedConnection?.id) {
      toast.danger("Select a Slack workspace")
      return
    }
    if (!selectedResource?.id) {
      toast.danger("Select a Slack channel")
      return
    }
    if (!selectedAgent?.id) {
      toast.danger("Select an agent")
      return
    }
    const trimmedInstructions = instructions.trim()
    if (!trimmedInstructions) {
      toast.danger("Instructions are required")
      return
    }
    const body = {
      provider: "slack",
      connection_id: selectedConnection.id,
      external_resource_key: selectedResource.id,
      external_resource_name: resourceName(selectedResource),
      agent_id: agentID,
      trigger_key: slackReactionKey,
      trigger_value: emojiName,
      instructions: trimmedInstructions,
    }
    const onSuccess = () => {
      toast.success(
        triggerID
          ? "Slack reaction trigger saved"
          : "Slack reaction trigger installed"
      )
      queryClient.invalidateQueries({ queryKey: ["get", "/v1/triggers"] })
      queryClient.invalidateQueries({ queryKey: ["get", "/v1/agents"] })
      queryClient.invalidateQueries({ queryKey: ["get", "/v1/channels"] })
      if (triggerID) {
        queryClient.invalidateQueries({
          queryKey: ["get", "/v1/triggers/{id}"],
        })
      }
      if (agentID) {
        queryClient.invalidateQueries({
          queryKey: ["get", "/v1/agents/{id}"],
        })
      }
      router.push("/w/automations")
    }
    const onError = (error: unknown) => {
      toast.danger(
        extractErrorMessage(
          error,
          triggerID ? "Could not save trigger" : "Could not install trigger"
        )
      )
    }

    if (triggerID) {
      updateTrigger.mutate(
        {
          params: { path: { id: triggerID } },
          body,
        },
        { onSuccess, onError }
      )
      return
    }

    createTrigger.mutate(
      {
        body,
      },
      { onSuccess, onError }
    )
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-6">
      {connections.length > 1 ? (
        <FormSection
          title="Slack workspace"
          description="Choose the connected Slack workspace that owns the channel."
        >
          <SlackConnectionSelect
            connections={connections}
            value={activeConnectionID}
            onChange={handleConnectionChange}
          />
        </FormSection>
      ) : null}

      <FormSection
        title="Slack channel"
        description={`Choose the ${automationSourceLabel(automation)} channel where this reaction should trigger the agent.`}
      >
        {connectionsQuery.isLoading ? (
          <FieldSkeleton />
        ) : connectionsQuery.isError ? (
          <InlineNotice
            icon="lucide:hash"
            title="Could not load Slack workspaces"
            body="Refresh the page and try again."
          />
        ) : connections.length === 0 ? (
          <InlineNotice
            icon="lucide:hash"
            title="No Slack connections"
            body="Connect Slack before installing this trigger."
          />
        ) : resourcesQuery.isLoading ? (
          <FieldSkeleton />
        ) : resourcesQuery.isError ? (
          <InlineNotice
            icon="lucide:hash"
            title="Could not load Slack channels"
            body="Refresh the channel list and try again."
          />
        ) : resources.length === 0 ? (
          <InlineNotice
            icon="lucide:hash"
            title="No Slack channels"
            body="No Slack channels were returned for this workspace."
          />
        ) : (
          <SlackResourceSelect
            resources={resources}
            value={resourceID}
            onChange={setResourceID}
          />
        )}
      </FormSection>

      <FormSection
        title="Agent"
        description="Select the agent that should handle matching Slack reactions."
      >
        {agents.length === 0 && !isLoading ? (
          <InlineNotice
            icon="lucide:bot"
            title="No active agents"
            body="Create or activate an agent before installing this trigger."
          />
        ) : (
          <AgentSelect
            agents={agents}
            selectedAgentID={agentID}
            isLoading={agentsQuery.isLoading}
            onChange={setAgentID}
          />
        )}
      </FormSection>

      <FormSection
        title="Reaction"
        description="The automation runs when someone adds this emoji to a Slack message."
      >
        <SlackEmojiPicker
          emojiName={emojiName}
          emojiGlyph={emojiGlyph}
          onChange={(name, glyph) => {
            setEmojiName(name)
            setEmojiGlyph(glyph)
          }}
        />
      </FormSection>

      <FormSection
        title="Instructions"
        description="These instructions are added to the agent run when the reaction event fires."
      >
        <textarea
          value={instructions}
          onChange={(event) => setInstructions(event.target.value)}
          rows={8}
          className="placeholder:text-muted-foreground min-h-44 w-full resize-y rounded-xl border border-border px-3 py-2.5 text-sm leading-5 text-foreground transition-colors outline-none focus:border-accent"
        />
      </FormSection>

      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-end">
        {existingTrigger ? (
          <p className="text-sm leading-5 text-warning">
            This agent already has a matching reaction trigger.
          </p>
        ) : null}
        <Button
          type="submit"
          variant="primary"
          size="sm"
          className="shrink-0"
          isDisabled={!canSubmit}
        >
          {createTrigger.isPending || updateTrigger.isPending ? (
            <Spinner color="current" size="sm" />
          ) : (
            <Icon
              icon={triggerID ? "lucide:save" : "lucide:plus"}
              className="h-4 w-4"
            />
          )}
          {triggerID ? "Save trigger" : "Install trigger"}
        </Button>
      </div>
    </form>
  )
}

function SlackConnectionSelect({
  connections,
  value,
  onChange,
}: {
  connections: Connection[]
  value: string
  onChange: (value: string) => void
}) {
  const selected = connections.find((connection) => connection.id === value)

  return (
    <Select
      aria-label="Slack workspace"
      selectedKey={value || null}
      onSelectionChange={(key) => {
        if (key !== null) onChange(String(key))
      }}
      className="w-full"
    >
      <Select.Trigger className="h-9 w-full justify-between rounded-md px-3 text-sm transition-colors">
        <span className="flex min-w-0 items-center gap-2">
          <Icon icon="simple-icons:slack" className="h-4 w-4 shrink-0" />
          <span className="truncate">
            {selected ? connectionLabel(selected) : "Select workspace"}
          </span>
        </span>
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover className="rounded-xl p-1.5">
        <ListBox>
          {connections.map((connection) => (
            <ListBox.Item
              key={connection.id}
              id={connection.id ?? ""}
              textValue={connectionLabel(connection)}
            >
              <span className="flex min-w-0 flex-col">
                <span className="truncate text-sm font-medium">
                  {connectionLabel(connection)}
                </span>
                <span className="text-muted-foreground truncate text-xs">
                  {connection.nango_connection_id}
                </span>
              </span>
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}

function SlackResourceSelect({
  resources,
  value,
  onChange,
}: {
  resources: AvailableResource[]
  value: string
  onChange: (value: string) => void
}) {
  const selected = resources.find((resource) => resource.id === value)

  return (
    <Select
      aria-label="Slack channel"
      selectedKey={value || null}
      onSelectionChange={(key) => {
        if (key !== null) onChange(String(key))
      }}
      className="w-full"
    >
      <Select.Trigger className="h-9 w-full justify-between rounded-md px-3 text-sm transition-colors">
        <span className="flex min-w-0 items-center gap-2">
          <Icon icon="lucide:hash" className="h-4 w-4 shrink-0 text-muted" />
          <span className="truncate">
            {selected ? resourceName(selected) : "Select channel"}
          </span>
        </span>
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover className="rounded-xl p-1.5">
        <ListBox>
          {resources.map((resource) => (
            <ListBox.Item
              key={resource.id}
              id={resource.id ?? ""}
              textValue={resourceName(resource)}
            >
              <span className="flex min-w-0 items-center gap-2">
                <Icon
                  icon="lucide:hash"
                  className="h-4 w-4 shrink-0 text-muted"
                />
                <span className="truncate text-sm font-medium">
                  {resourceName(resource)}
                </span>
              </span>
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}

function connectionLabel(connection: Connection): string {
  return (
    connection.display_name ||
    connection.nango_connection_id ||
    connection.id ||
    "Slack"
  )
}

function resourceName(resource: AvailableResource): string {
  return resource.name || resource.id || "Slack channel"
}

function triggerSourceSlug(
  provider: string,
  key: string,
  resourceKey: string,
  value: string
): string {
  return [provider, key, resourceKey, value].join(":")
}
