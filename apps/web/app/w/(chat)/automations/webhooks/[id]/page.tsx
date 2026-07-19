"use client"

import { useState } from "react"
import { useParams, useRouter } from "next/navigation"
import NextLink from "next/link"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Input, Spinner, Switch, TextArea, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { queryKeys } from "@/lib/api/query-keys"
import { useIsAdmin } from "@/lib/auth/use-role"
import { resolveTeamAgentID, useTeamAgents } from "@/lib/api/team-agents"
import { AgentSelect } from "@/components/agent-select"
import { TeamSelect, useTeams } from "@/app/w/(chat)/automations/_team-select"
import {
  FormSection,
  InlineNotice,
} from "@/app/w/(chat)/automations/_trigger-form-sections"
import { TriggerDeleteConfirmModal } from "@/app/w/(chat)/automations/_trigger-delete-confirm-modal"
import type { InstalledTrigger } from "@/app/w/(chat)/automations/_data"

export default function WebhookTriggerDetailPage() {
  const params = useParams<{ id: string }>()
  const id = params.id
  const router = useRouter()

  const triggerQuery = $api.useQuery(
    "get",
    "/v1/triggers/{id}",
    { params: { path: { id } } },
    { retry: false }
  )
  const trigger = triggerQuery.data?.trigger

  return (
    <div className="h-full overflow-y-auto bg-background text-foreground">
      <div className="mx-auto w-full max-w-2xl px-6 py-12">
        <div className="flex flex-col gap-8">
          <button
            type="button"
            onClick={() => router.push("/w/automations?tab=webhooks")}
            className="text-muted-foreground flex w-fit items-center gap-1.5 text-sm transition-colors hover:text-foreground"
          >
            <AppIcon icon="arrow-left" className="h-4 w-4" />
            Webhooks
          </button>

          {triggerQuery.isLoading ? (
            <div className="flex min-h-56 items-center justify-center">
              <Spinner />
            </div>
          ) : triggerQuery.isError || !trigger ? (
            <div className="bg-card flex min-h-56 flex-col items-center justify-center rounded-xl px-6 text-center">
              <AppIcon icon="triangle-alert" className="h-7 w-7 text-muted" />
              <p className="mt-3 text-sm font-medium text-foreground">
                Webhook trigger not found
              </p>
            </div>
          ) : (
            <WebhookEditForm key={trigger.id} trigger={trigger} />
          )}
        </div>
      </div>
    </div>
  )
}

function WebhookEditForm({ trigger }: { trigger: InstalledTrigger }) {
  const router = useRouter()
  const queryClient = useQueryClient()
  const id = trigger.id ?? ""

  // Editing an existing webhook trigger (toggle enabled, save, delete)
  // mutates via PATCH/DELETE /v1/triggers/{id}, which is admin-only on the
  // backend. Creating a trigger is a member action and isn't gated here.
  const isAdmin = useIsAdmin()
  const { teams, isLoading: teamsLoading } = useTeams()
  const updateTrigger = $api.useMutation("patch", "/v1/triggers/{id}")
  const deleteTrigger = $api.useMutation("delete", "/v1/triggers/{id}")

  const [name, setName] = useState(trigger.name ?? "")
  const [teamID, setTeamID] = useState("")
  const [agentID, setAgentID] = useState(trigger.agent_id ?? "")
  const [instructions, setInstructions] = useState(trigger.instructions ?? "")
  const [deleteOpen, setDeleteOpen] = useState(false)

  const activeTeamID = teamID || teams[0]?.id || ""
  const { agents, isLoading: agentsLoading } = useTeamAgents(activeTeamID)
  const activeAgentID = resolveTeamAgentID(agents, agentID, undefined)

  const enabled = trigger.enabled ?? true
  const lastRunHref = trigger.last_run_session_id
    ? `/w/sessions/${trigger.last_run_session_id}`
    : null

  const canSave = Boolean(
    isAdmin &&
    !updateTrigger.isPending &&
    name.trim() &&
    activeTeamID &&
    activeAgentID &&
    instructions.trim()
  )

  function invalidate() {
    queryClient.invalidateQueries({ queryKey: queryKeys.triggers() })
    queryClient.invalidateQueries({ queryKey: queryKeys.trigger() })
  }

  function handleSave() {
    if (!name.trim() || !activeTeamID || !activeAgentID) {
      toast.danger("Fill in name, team and agent")
      return
    }
    if (!instructions.trim()) {
      toast.danger("Instructions are required")
      return
    }
    updateTrigger.mutate(
      {
        params: { path: { id } },
        body: {
          name: name.trim(),
          agent_id: activeAgentID,
          instructions: instructions.trim(),
        },
      },
      {
        onSuccess: () => {
          toast.success("Webhook saved")
          invalidate()
        },
        onError: (error) =>
          toast.danger(extractErrorMessage(error, "Could not save webhook")),
      }
    )
  }

  function toggleEnabled(next: boolean) {
    updateTrigger.mutate(
      { params: { path: { id } }, body: { enabled: next } },
      {
        onSuccess: () => {
          toast.success(next ? "Webhook enabled" : "Webhook disabled")
          invalidate()
        },
        onError: (error) =>
          toast.danger(extractErrorMessage(error, "Could not update webhook")),
      }
    )
  }

  function confirmDelete() {
    deleteTrigger.mutate(
      { params: { path: { id } } },
      {
        onSuccess: () => {
          setDeleteOpen(false)
          toast.success("Webhook trigger deleted")
          invalidate()
          router.push("/w/automations?tab=webhooks")
        },
        onError: (error) =>
          toast.danger(extractErrorMessage(error, "Could not delete webhook")),
      }
    )
  }

  async function copyUrl() {
    if (!trigger.webhook_url) return
    try {
      await navigator.clipboard.writeText(trigger.webhook_url)
      toast.success("Webhook URL copied")
    } catch {
      toast.danger("Could not copy URL")
    }
  }

  return (
    <>
      <header className="flex items-center gap-3">
        <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl bg-sky-500 text-white">
          <AppIcon icon="globe" className="h-6 w-6" />
        </div>
        <div className="min-w-0">
          <h1 className="truncate text-lg font-semibold text-foreground">
            {trigger.name || trigger.agent_name || "Webhook trigger"}
          </h1>
          <p className="text-muted-foreground mt-1 text-sm">
            Edit this webhook trigger.
          </p>
        </div>
      </header>

      <div className="flex gap-2.5 rounded-xl border border-sky-500/25 bg-sky-500/[0.06] px-3.5 py-3">
        <AppIcon
          icon="clock"
          className="mt-0.5 h-4 w-4 shrink-0 text-sky-500"
        />
        <div className="flex flex-col gap-1 text-sm leading-5">
          <span className="font-medium text-foreground">
            {trigger.last_run_at
              ? `Last run ${formatDateTime(trigger.last_run_at)}`
              : "Hasn't run yet"}
          </span>
          <span className="text-muted-foreground">
            {trigger.last_run_at ? (
              lastRunHref ? (
                <NextLink
                  href={lastRunHref}
                  className="font-medium text-sky-600 underline underline-offset-2 hover:text-sky-500"
                >
                  View session
                </NextLink>
              ) : (
                "The most recent run's session is no longer available."
              )
            ) : (
              "This webhook hasn't received a request yet."
            )}
          </span>
        </div>
      </div>

      <section className="flex flex-col gap-3">
        <h2 className="text-sm font-semibold text-foreground">Webhook URL</h2>
        <div className="bg-card flex items-center gap-2 rounded-xl border border-border px-3 py-2.5">
          <code className="min-w-0 flex-1 truncate font-mono text-sm text-foreground">
            {trigger.webhook_url}
          </code>
          <Button
            type="button"
            variant="secondary"
            size="sm"
            className="shrink-0"
            onPress={copyUrl}
          >
            <AppIcon icon="copy" className="h-4 w-4" />
            Copy
          </Button>
        </div>
        <p className="text-muted-foreground text-sm leading-5">
          Send a POST request to this URL to run the agent.
          {trigger.secret_set
            ? " Requests must include the shared secret in the Authorization header."
            : " This webhook is unauthenticated — anyone with the URL can trigger it."}
        </p>
      </section>

      <form
        onSubmit={(event) => {
          event.preventDefault()
          handleSave()
        }}
        className="flex flex-col gap-6"
      >
        <FormSection
          title="Name"
          description="A label for this webhook, shown in your automations list."
        >
          <Input
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder="Name this webhook"
            className="h-9 w-full rounded-md"
          />
        </FormSection>

        <FormSection
          title="Team"
          description="The team that owns the agent handling this webhook."
        >
          {teamsLoading ? (
            <div className="h-9 animate-pulse rounded-md bg-default" />
          ) : teams.length === 0 ? (
            <InlineNotice
              icon="hash"
              title="No teams"
              body="No teams available."
            />
          ) : (
            <TeamSelect
              teams={teams}
              value={activeTeamID}
              onChange={setTeamID}
            />
          )}
        </FormSection>

        <FormSection
          title="Agent"
          description="The agent that handles inbound webhook requests."
        >
          {!activeTeamID ? (
            <InlineNotice
              icon="bot"
              title="Select a team first"
              body="Agents are scoped to teams."
            />
          ) : agents.length === 0 && !agentsLoading ? (
            <InlineNotice
              icon="bot"
              title="No agents on this team"
              body="Add an agent to this team to handle this webhook."
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
          title="Instructions"
          description="These instructions are added to the agent run when the webhook URL is called."
        >
          <TextArea
            value={instructions}
            onChange={(event) => setInstructions(event.target.value)}
            rows={8}
            fullWidth
            className="min-h-44 resize-y leading-5"
          />
        </FormSection>

        <FormSection
          title="Status"
          description={
            enabled
              ? "Incoming requests will run this agent."
              : "Incoming requests are ignored."
          }
        >
          <div className="flex items-center justify-between gap-4 rounded-xl border border-border px-3 py-2.5">
            <span className="text-sm font-medium text-foreground">
              {enabled ? "Enabled" : "Disabled"}
            </span>
            <Switch
              aria-label="Enable webhook"
              isSelected={enabled}
              isDisabled={updateTrigger.isPending || !isAdmin}
              onChange={toggleEnabled}
              className="shrink-0"
            >
              <Switch.Control>
                <Switch.Thumb />
              </Switch.Control>
            </Switch>
          </div>
        </FormSection>

        {!isAdmin ? (
          <p className="text-muted-foreground text-sm">
            Only workspace admins can edit or delete automations.
          </p>
        ) : null}

        <div className="flex items-center justify-between gap-3">
          <Button
            type="button"
            variant="secondary"
            size="sm"
            className="text-danger"
            isDisabled={deleteTrigger.isPending || !isAdmin}
            onPress={() => setDeleteOpen(true)}
          >
            {deleteTrigger.isPending ? (
              <Spinner color="current" size="sm" />
            ) : (
              <AppIcon icon="trash-2" className="h-4 w-4" />
            )}
            Delete
          </Button>
          <Button
            type="submit"
            variant="primary"
            size="sm"
            isDisabled={!canSave}
          >
            {updateTrigger.isPending ? (
              <Spinner color="current" size="sm" />
            ) : (
              <AppIcon icon="check" className="h-4 w-4" />
            )}
            Save changes
          </Button>
        </div>
      </form>

      <TriggerDeleteConfirmModal
        open={deleteOpen}
        pending={deleteTrigger.isPending}
        onOpenChange={setDeleteOpen}
        onConfirm={confirmDelete}
      />
    </>
  )
}

function formatDateTime(value?: string): string {
  if (!value) return "—"
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return "—"
  return date.toLocaleString(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  })
}
