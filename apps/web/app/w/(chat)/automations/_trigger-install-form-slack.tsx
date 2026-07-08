"use client"

import { FormEvent, useMemo, useState } from "react"
import { useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Input, Spinner, Switch, TextArea, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { queryKeys } from "@/lib/api/query-keys"
import { useIsAdmin } from "@/lib/auth/use-role"
import {
  automationSourceLabel,
  automationTriggerDefaultInstructions,
  automationTriggerDefaultValue,
  type AutomationItem,
  type InstalledTrigger,
} from "@/app/w/(chat)/automations/_data"
import { AgentSelect } from "@/components/agent-select"
import {
  defaultEmojiGlyph,
  normalizeEmojiName,
  SlackEmojiPicker,
} from "@/app/w/(chat)/automations/_slack-emoji-picker"
import {
  resourceName,
  SlackConnectionSelect,
  SlackResourceSelect,
} from "@/app/w/(chat)/automations/_slack-resource-select"
import {
  FieldSkeleton,
  FormSection,
  InlineNotice,
} from "@/app/w/(chat)/automations/_trigger-form-sections"
import { TriggerDeleteConfirmModal } from "@/app/w/(chat)/automations/_trigger-delete-confirm-modal"
import {
  type AvailableResource,
  slackChannelResourceType,
  slackReactionKey,
  triggerSourceSlug,
} from "@/app/w/(chat)/automations/_trigger-install-form-shared"

export function SlackReactionInstallForm({
  automation,
  trigger,
}: {
  automation: AutomationItem
  trigger?: InstalledTrigger
}) {
  const router = useRouter()
  const queryClient = useQueryClient()
  const triggerID = trigger?.id || ""
  // Editing an existing trigger (save, toggle, delete) mutates via
  // PATCH/DELETE /v1/triggers/{id}, which is admin-only on the backend.
  // Installing a new trigger (triggerID empty) is a member action and isn't
  // gated here.
  const isAdmin = useIsAdmin()
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
  const deleteTrigger = $api.useMutation("delete", "/v1/triggers/{id}")
  const defaultEmojiName = normalizeEmojiName(
    trigger?.trigger_value ||
      automationTriggerDefaultValue(automation) ||
      "eyes"
  )
  const [name, setName] = useState(trigger?.name || automation.name || "")
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
  const [isEnabled, setIsEnabled] = useState(trigger?.enabled ?? true)
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false)

  const connections = useMemo(
    () => connectionsQuery.data?.data ?? [],
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
  const triggerResourceKey = trigger?.external_resource_key
  const triggerResourceName = trigger?.external_resource_name
  const initialResource = useMemo(() => {
    if (!triggerResourceKey) return null
    return {
      id: triggerResourceKey,
      name: triggerResourceName || triggerResourceKey,
      type: slackChannelResourceType,
    } satisfies AvailableResource
  }, [triggerResourceKey, triggerResourceName])
  const resources = useMemo(() => {
    const list = (resourcesQuery.data?.resources ?? []).filter((resource) =>
      Boolean(resource.id)
    )
    if (
      initialResource?.id &&
      !list.some((resource) => resource.id === initialResource.id)
    ) {
      return [initialResource, ...list]
    }
    return list
  }, [initialResource, resourcesQuery.data?.resources])
  const activeResourceID = resourceID || resources[0]?.id || ""
  const selectedResource = useMemo(
    () => resources.find((resource) => resource.id === activeResourceID),
    [activeResourceID, resources]
  )
  const agents = useMemo(
    () => (agentsQuery.data?.data ?? []).filter((agent) => agent.id),
    [agentsQuery.data?.data]
  )
  const activeAgentID = agents.some((agent) => agent.id === agentID)
    ? agentID
    : (agents[0]?.id ?? "")
  const selectedAgent = useMemo(
    () => agents.find((agent) => agent.id === activeAgentID),
    [activeAgentID, agents]
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

  const isLoading =
    connectionsQuery.isLoading ||
    resourcesQuery.isLoading ||
    agentsQuery.isLoading
  const isSaving = createTrigger.isPending || updateTrigger.isPending
  const isBusy = isSaving || deleteTrigger.isPending
  const canSubmit =
    (!triggerID || isAdmin) &&
    !isLoading &&
    !isBusy &&
    !existingTrigger &&
    Boolean(
      name.trim() &&
      activeConnectionID &&
      selectedResource?.id &&
      activeAgentID &&
      emojiName &&
      instructions.trim()
    )

  function handleConnectionChange(id: string) {
    setConnectionID(id)
    setResourceID("")
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!name.trim()) {
      toast.danger("Name is required")
      return
    }
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
      name: name.trim(),
      provider: "slack",
      connection_id: selectedConnection.id,
      external_resource_key: selectedResource.id,
      external_resource_name: resourceName(selectedResource),
      agent_id: activeAgentID,
      trigger_key: slackReactionKey,
      trigger_value: emojiName,
      instructions: trimmedInstructions,
    }
    const updateBody = { ...body, enabled: isEnabled }
    const onSuccess = () => {
      toast.success(
        triggerID
          ? "Slack reaction trigger saved"
          : "Slack reaction trigger installed"
      )
      queryClient.invalidateQueries({ queryKey: queryKeys.triggers() })
      queryClient.invalidateQueries({ queryKey: queryKeys.agents() })
      queryClient.invalidateQueries({ queryKey: queryKeys.channels() })
      if (triggerID) {
        queryClient.invalidateQueries({
          queryKey: queryKeys.trigger(),
        })
      }
      if (activeAgentID) {
        queryClient.invalidateQueries({
          queryKey: queryKeys.agent(),
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
          body: updateBody,
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

  function handleDelete() {
    if (!triggerID) return
    setDeleteConfirmOpen(true)
  }

  function confirmDelete() {
    if (!triggerID) return
    deleteTrigger.mutate(
      { params: { path: { id: triggerID } } },
      {
        onSuccess: () => {
          setDeleteConfirmOpen(false)
          toast.success("Trigger deleted")
          queryClient.invalidateQueries({ queryKey: queryKeys.triggers() })
          queryClient.invalidateQueries({ queryKey: queryKeys.agents() })
          queryClient.invalidateQueries({ queryKey: queryKeys.channels() })
          router.push("/w/automations")
        },
        onError: (error) =>
          toast.danger(extractErrorMessage(error, "Could not delete trigger")),
      }
    )
  }

  return (
    <>
      <form onSubmit={handleSubmit} className="flex flex-col gap-6">
        <FormSection
          title="Name"
          description="A label for this trigger, shown in your automations list."
        >
          <Input
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder="Name this trigger"
            className="h-9 w-full rounded-md"
          />
        </FormSection>

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
              icon="hash"
              title="Could not load Slack workspaces"
              body="Refresh the page and try again."
            />
          ) : connections.length === 0 ? (
            <InlineNotice
              icon="hash"
              title="No Slack connections"
              body="Connect Slack before installing this trigger."
            />
          ) : resourcesQuery.isLoading ? (
            <FieldSkeleton />
          ) : resourcesQuery.isError ? (
            <InlineNotice
              icon="hash"
              title="Could not load Slack channels"
              body="Refresh the channel list and try again."
            />
          ) : resources.length === 0 ? (
            <InlineNotice
              icon="hash"
              title="No Slack channels"
              body="No Slack channels were returned for this workspace."
            />
          ) : (
            <SlackResourceSelect
              resources={resources}
              value={activeResourceID}
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
              icon="bot"
              title="No active agents"
              body="Create or activate an agent before installing this trigger."
            />
          ) : (
            <AgentSelect
              agents={agents}
              selectedAgentID={activeAgentID}
              isLoading={agentsQuery.isLoading}
              onChange={setAgentID}
              variant="field"
            />
          )}
        </FormSection>

        {triggerID ? (
          <FormSection
            title="Status"
            description="Disable this trigger without removing its configuration."
          >
            <div className="flex items-center justify-between gap-4 rounded-xl border border-border px-3 py-2.5">
              <div className="flex min-w-0 flex-col gap-0.5">
                <span className="text-sm font-medium text-foreground">
                  {isEnabled ? "Enabled" : "Disabled"}
                </span>
                <span className="text-muted-foreground text-sm leading-5">
                  {isEnabled
                    ? "Matching Slack reactions will run this automation."
                    : "Matching Slack reactions will be ignored."}
                </span>
              </div>
              <Switch
                aria-label="Enable trigger"
                isSelected={isEnabled}
                isDisabled={isBusy || !isAdmin}
                onChange={setIsEnabled}
                className="shrink-0"
              >
                <Switch.Control>
                  <Switch.Thumb />
                </Switch.Control>
              </Switch>
            </div>
          </FormSection>
        ) : null}

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
          <TextArea
            value={instructions}
            onChange={(event) => setInstructions(event.target.value)}
            rows={8}
            fullWidth
            className="min-h-44 resize-y leading-5"
          />
        </FormSection>

        {triggerID && !isAdmin ? (
          <p className="text-muted-foreground text-sm">
            Only workspace admins can edit or delete automations.
          </p>
        ) : null}

        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-end">
          {triggerID ? (
            <Button
              type="button"
              variant="secondary"
              size="sm"
              className="text-danger sm:mr-auto"
              isDisabled={isBusy || !isAdmin}
              onPress={handleDelete}
            >
              {deleteTrigger.isPending ? (
                <Spinner color="current" size="sm" />
              ) : (
                <AppIcon icon="trash-2" className="h-4 w-4" />
              )}
              Delete trigger
            </Button>
          ) : null}
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
            {isSaving ? (
              <Spinner color="current" size="sm" />
            ) : (
              <AppIcon icon={triggerID ? "save" : "plus"} className="h-4 w-4" />
            )}
            {triggerID ? "Save trigger" : "Install trigger"}
          </Button>
        </div>
      </form>
      {triggerID ? (
        <TriggerDeleteConfirmModal
          open={deleteConfirmOpen}
          pending={deleteTrigger.isPending}
          onOpenChange={setDeleteConfirmOpen}
          onConfirm={confirmDelete}
        />
      ) : null}
    </>
  )
}
