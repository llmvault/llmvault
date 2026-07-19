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
  automationTriggerDefaultInstructions,
  automationTriggerKey,
  type AutomationItem,
  type InstalledTrigger,
} from "@/app/w/(chat)/automations/_data"
import { AgentSelect } from "@/components/agent-select"
import { resolveTeamAgentID, useTeamAgents } from "@/lib/api/team-agents"
import { TeamSelect, useTeams } from "@/app/w/(chat)/automations/_team-select"
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
  teamDescription?: string
  statusEnabledDescription?: string
  statusDisabledDescription?: string
}

const defaultTeamDescription =
  "Choose the team that owns the agent this automation runs."
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
  // Editing an existing trigger (save, toggle, delete) mutates via
  // PATCH/DELETE /v1/triggers/{id}, which is admin-only on the backend.
  // Installing a new trigger (triggerID empty) is a member action and isn't
  // gated here.
  const isAdmin = useIsAdmin()
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
  const { teams, isLoading: teamsLoading } = useTeams()
  const [name, setName] = useState(trigger?.name || automation.name || "")
  const [teamID, setTeamID] = useState("")
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
  const activeTeamID = teamID || teams[0]?.id || ""
  const { agents, isLoading: agentsLoading } = useTeamAgents(
    activeTeamID
  )
  const activeAgentID = resolveTeamAgentID(
    agents,
    agentID,
    undefined
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
    teamsLoading
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
      activeTeamID &&
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
    if (!activeTeamID) {
      toast.danger("Select a team")
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
      queryClient.invalidateQueries({ queryKey: queryKeys.triggers() })
      queryClient.invalidateQueries({ queryKey: queryKeys.agents() })
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
          {!activeTeamID ? (
            <InlineNotice
              icon="bot"
              title="Select a team first"
              body="Agents are scoped to a team."
            />
          ) : agents.length === 0 && !agentsLoading ? (
            <InlineNotice
              icon="bot"
              title="No agents on this team"
              body="Add an agent to this team before installing this trigger."
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
          title="Team"
          description={config.teamDescription ?? defaultTeamDescription}
        >
          {teamsLoading ? (
            <FieldSkeleton />
          ) : teams.length === 0 ? (
            <InlineNotice
              icon="hash"
              title="No teams"
              body="Create a team before installing this trigger."
            />
          ) : (
            <TeamSelect
              teams={teams}
              value={activeTeamID}
              onChange={setTeamID}
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
