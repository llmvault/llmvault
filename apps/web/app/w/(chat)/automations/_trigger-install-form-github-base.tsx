"use client"

import { FormEvent, useMemo, useState } from "react"
import { useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Input, Spinner, Switch, TextArea, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import {
  automationTriggerDefaultInstructions,
  automationTriggerKey,
  type AutomationItem,
  type InstalledTrigger,
} from "@/app/w/(chat)/automations/_data"
import { AgentSelect } from "@/components/agent-select"
import {
  resolveScopedAgentID,
  useChannelScopedAgents,
} from "@/lib/api/channel-scoped-agents"
import {
  ChannelSelect,
  useHivyChannels,
} from "@/app/w/(chat)/automations/_channel-select"
import {
  GithubConnectionSelect,
  GithubRepoSelect,
  repoName,
} from "@/app/w/(chat)/automations/_github-resource-select"
import {
  FieldSkeleton,
  FormSection,
  InlineNotice,
} from "@/app/w/(chat)/automations/_trigger-form-sections"
import { TriggerDeleteConfirmModal } from "@/app/w/(chat)/automations/_trigger-delete-confirm-modal"
import {
  type AvailableResource,
  type Connection,
  githubRepoResourceType,
  triggerSourceSlug,
} from "@/app/w/(chat)/automations/_trigger-install-form-shared"

// GithubMentionFormConfig captures everything that differs between the two
// GitHub App mention forms: the connection provider they bind to and the copy
// that names the mentioned bot. The form body itself is identical, so both
// _trigger-install-form-github.tsx and _trigger-install-form-github-code-reviews.tsx
// are thin wrappers that pass a config here.
export type GithubMentionFormConfig = {
  // provider is the connection provider both the connections query and the
  // stored trigger row bind to (e.g. github-app vs github-app-code-reviews).
  provider: string
  // toastNoun labels the install/save success toasts, e.g. "GitHub mention
  // trigger" or "GitHub code review trigger".
  toastNoun: string
  repoDescription: string
  agentDescription: string
  instructionsDescription: string
  existingWarning: string
  // The following copy differs between the mention forms (which talk about
  // @mentions) and the auto-review form (which fires on every PR, no mention).
  // Each is optional and falls back to the mention wording.
  channelDescription?: string
  statusEnabledDescription?: string
  statusDisabledDescription?: string
}

const defaultChannelDescription =
  "The channel this agent's session runs in when it's @mentioned. You can only pick channels you have access to."
const defaultStatusEnabledDescription =
  "Mentions in this repository will run this automation."
const defaultStatusDisabledDescription =
  "Mentions in this repository will be ignored."

export function GithubMentionInstallFormBase({
  automation,
  trigger,
  config,
}: {
  automation: AutomationItem
  trigger?: InstalledTrigger
  config: GithubMentionFormConfig
}) {
  const router = useRouter()
  const queryClient = useQueryClient()
  const triggerID = trigger?.id || ""
  // The mention family shares this form; install with the catalog item's own key
  // (mention | issue_mention | pr_mention), not a hardcoded one.
  const triggerKey = trigger?.trigger_key || automationTriggerKey(automation)
  const connectionsQuery = $api.useQuery(
    "get",
    "/v1/connections",
    {
      params: { query: { provider: config.provider, limit: 100 } },
    },
    { retry: false }
  )
  const createTrigger = $api.useMutation("post", "/v1/triggers")
  const updateTrigger = $api.useMutation("patch", "/v1/triggers/{id}")
  const deleteTrigger = $api.useMutation("delete", "/v1/triggers/{id}")
  const { channels, isLoading: channelsLoading } = useHivyChannels()
  const [name, setName] = useState(trigger?.name || automation.name || "")
  const [channelID, setChannelID] = useState(trigger?.channel_id || "")
  const [connectionID, setConnectionID] = useState(trigger?.connection_id || "")
  const [resourceID, setResourceID] = useState(
    trigger?.external_resource_key || ""
  )
  const [agentID, setAgentID] = useState(trigger?.agent_id || "")
  const [instructions, setInstructions] = useState(
    trigger?.instructions || automationTriggerDefaultInstructions(automation)
  )
  const [isEnabled, setIsEnabled] = useState(trigger?.enabled ?? true)
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false)

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
          type: githubRepoResourceType,
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
      type: githubRepoResourceType,
    } satisfies AvailableResource
  }, [triggerResourceKey, triggerResourceName])
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
  const activeResourceID = resourceID || resources[0]?.id || ""
  const selectedResource = useMemo(
    () => resources.find((resource) => resource.id === activeResourceID),
    [activeResourceID, resources]
  )
  const activeChannelID = channelID || channels[0]?.id || ""
  const activeChannel = channels.find((c) => c.id === activeChannelID)
  const { agents, isLoading: agentsLoading } =
    useChannelScopedAgents(activeChannelID)
  const activeAgentID = resolveScopedAgentID(
    agents,
    agentID,
    activeChannel?.default_agent_id
  )
  const selectedAgent = useMemo(
    () => agents.find((agent) => agent.id === activeAgentID),
    [activeAgentID, agents]
  )
  const existingTrigger = useMemo(
    () =>
      selectedAgent?.triggers?.some(
        (item) =>
          item.id !== triggerID &&
          item.provider === config.provider &&
          item.connection_id === activeConnectionID &&
          item.trigger_key === triggerKey &&
          item.source_slug ===
            triggerSourceSlug(
              config.provider,
              triggerKey,
              selectedResource?.id ?? "",
              (selectedResource?.id ?? "").toLowerCase()
            )
      ) ?? false,
    [
      activeConnectionID,
      config.provider,
      selectedAgent,
      selectedResource?.id,
      triggerID,
      triggerKey,
    ]
  )

  const isLoading =
    connectionsQuery.isLoading ||
    resourcesQuery.isLoading ||
    agentsLoading ||
    channelsLoading
  const isSaving = createTrigger.isPending || updateTrigger.isPending
  const isBusy = isSaving || deleteTrigger.isPending
  const canSubmit =
    !isLoading &&
    !isBusy &&
    !existingTrigger &&
    Boolean(
      name.trim() &&
      activeConnectionID &&
      selectedResource?.id &&
      activeAgentID &&
      activeChannelID &&
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
      toast.danger("Select a GitHub connection")
      return
    }
    if (!selectedResource?.id) {
      toast.danger("Select a repository")
      return
    }
    if (!selectedAgent?.id) {
      toast.danger("Select an agent")
      return
    }
    if (!activeChannelID) {
      toast.danger("Select a channel")
      return
    }
    const trimmedInstructions = instructions.trim()
    if (!trimmedInstructions) {
      toast.danger("Instructions are required")
      return
    }
    const body = {
      name: name.trim(),
      provider: config.provider,
      connection_id: selectedConnection.id,
      external_resource_key: selectedResource.id,
      external_resource_name: repoName(selectedResource),
      agent_id: activeAgentID,
      channel_id: activeChannelID,
      trigger_key: triggerKey,
      instructions: trimmedInstructions,
    }
    const updateBody = { ...body, enabled: isEnabled }
    const onSuccess = () => {
      toast.success(
        triggerID
          ? `${config.toastNoun} saved`
          : `${config.toastNoun} installed`
      )
      queryClient.invalidateQueries({ queryKey: ["get", "/v1/triggers"] })
      queryClient.invalidateQueries({ queryKey: ["get", "/v1/agents"] })
      queryClient.invalidateQueries({ queryKey: ["get", "/v1/channels"] })
      if (triggerID) {
        queryClient.invalidateQueries({
          queryKey: ["get", "/v1/triggers/{id}"],
        })
      }
      if (activeAgentID) {
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
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/triggers"] })
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/agents"] })
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/channels"] })
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
            title="GitHub connection"
            description="Choose the GitHub App installation that owns the repository."
          >
            <GithubConnectionSelect
              connections={connections}
              value={activeConnectionID}
              onChange={handleConnectionChange}
            />
          </FormSection>
        ) : null}

        <FormSection title="Repository" description={config.repoDescription}>
          {connectionsQuery.isLoading ? (
            <FieldSkeleton />
          ) : connectionsQuery.isError ? (
            <InlineNotice
              icon="git-branch"
              title="Could not load GitHub connections"
              body="Refresh the page and try again."
            />
          ) : connections.length === 0 ? (
            <InlineNotice
              icon="git-branch"
              title="No GitHub connections"
              body="Connect GitHub before installing this trigger."
            />
          ) : resourcesQuery.isLoading ? (
            <FieldSkeleton />
          ) : resourcesQuery.isError ? (
            <InlineNotice
              icon="git-branch"
              title="Could not load repositories"
              body="Refresh the repository list and try again."
            />
          ) : resources.length === 0 ? (
            <InlineNotice
              icon="git-branch"
              title="No repositories"
              body="The GitHub App installation has no accessible repositories."
            />
          ) : (
            <GithubRepoSelect
              resources={resources}
              value={activeResourceID}
              onChange={setResourceID}
            />
          )}
        </FormSection>

        <FormSection title="Agent" description={config.agentDescription}>
          {!activeChannelID ? (
            <InlineNotice
              icon="bot"
              title="Select a channel first"
              body="Agents are scoped to the channel this trigger runs in."
            />
          ) : agents.length === 0 && !agentsLoading ? (
            <InlineNotice
              icon="bot"
              title="No agents in this channel"
              body="Assign an agent to the selected channel before installing this trigger."
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
          title="Channel"
          description={config.channelDescription ?? defaultChannelDescription}
        >
          {channelsLoading ? (
            <FieldSkeleton />
          ) : channels.length === 0 ? (
            <InlineNotice
              icon="hash"
              title="No channels"
              body="Create a channel before installing this trigger."
            />
          ) : (
            <ChannelSelect
              channels={channels}
              value={activeChannelID}
              onChange={setChannelID}
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
                    ? (config.statusEnabledDescription ??
                      defaultStatusEnabledDescription)
                    : (config.statusDisabledDescription ??
                      defaultStatusDisabledDescription)}
                </span>
              </div>
              <Switch
                aria-label="Enable trigger"
                isSelected={isEnabled}
                isDisabled={isBusy}
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
          title="Instructions"
          description={config.instructionsDescription}
        >
          <TextArea
            value={instructions}
            onChange={(event) => setInstructions(event.target.value)}
            rows={8}
            fullWidth
            className="min-h-44 resize-y leading-5"
          />
        </FormSection>

        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-end">
          {triggerID ? (
            <Button
              type="button"
              variant="secondary"
              size="sm"
              className="text-danger sm:mr-auto"
              isDisabled={isBusy}
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
              {config.existingWarning}
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
              <AppIcon
                icon={triggerID ? "save" : "plus"}
                className="h-4 w-4"
              />
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
