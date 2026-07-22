"use client"

import { FormEvent, useState } from "react"
import { useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Input, Spinner, TextArea, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { queryKeys } from "@/lib/api/query-keys"
import { resolveTeamAgentID, useTeamAgents } from "@/lib/api/team-agents"
import { AgentSelect } from "@/components/agent-select"
import { TeamSelect, useTeams } from "@/app/w/(chat)/automations/_team-select"
import {
  FormSection,
  InlineNotice,
} from "@/app/w/(chat)/automations/_trigger-form-sections"
import { TutorialBanner } from "@/components/tutorial-banner"

const DEFAULT_INSTRUCTIONS =
  "You were triggered by an inbound webhook request. Read the request payload, decide what action it calls for, and carry it out. If there isn't enough context to act confidently, summarise what you received instead of guessing."

export default function NewWebhookTriggerPage() {
  const router = useRouter()
  const queryClient = useQueryClient()

  const { teams, isLoading: teamsLoading } = useTeams()
  const createTrigger = $api.useMutation("post", "/v1/triggers")

  const [name, setName] = useState("")
  const [teamID, setTeamID] = useState("")
  const [agentID, setAgentID] = useState("")
  const [instructions, setInstructions] = useState(DEFAULT_INSTRUCTIONS)
  const [secret, setSecret] = useState("")

  const activeTeamID = teamID || teams[0]?.id || ""
  const { agents, isLoading: agentsLoading } = useTeamAgents(activeTeamID)
  const activeAgentID = resolveTeamAgentID(agents, agentID, undefined)

  const isSaving = createTrigger.isPending
  const canSubmit = Boolean(
    !isSaving &&
    name.trim() &&
    activeTeamID &&
    activeAgentID &&
    instructions.trim()
  )

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!name.trim()) {
      toast.danger("Name is required")
      return
    }
    if (!activeTeamID) {
      toast.danger("Select a team")
      return
    }
    if (!activeAgentID) {
      toast.danger("Select an agent")
      return
    }
    const trimmed = instructions.trim()
    if (!trimmed) {
      toast.danger("Instructions are required")
      return
    }
    createTrigger.mutate(
      {
        body: {
          trigger_type: "http",
          name: name.trim(),
          agent_id: activeAgentID,
          instructions: trimmed,
          secret_key: secret.trim() || undefined,
        },
      },
      {
        onSuccess: () => {
          toast.success("Webhook trigger created")
          queryClient.invalidateQueries({ queryKey: queryKeys.triggers() })
          router.push("/w/automations?tab=webhooks")
        },
        onError: (error) =>
          toast.danger(
            extractErrorMessage(error, "Could not create webhook trigger")
          ),
      }
    )
  }

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

          <header className="flex items-center gap-3">
            <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl bg-sky-500 text-white">
              <AppIcon icon="globe" className="h-6 w-6" />
            </div>
            <div>
              <h1 className="text-lg font-semibold text-foreground">
                Add webhook trigger
              </h1>
              <p className="text-muted-foreground mt-1 text-sm">
                Run an agent when an inbound HTTP request hits a unique URL.
              </p>
            </div>
          </header>

          <TutorialBanner
            tutorial="webhooks"
            title="Create a secure webhook trigger"
            description="See how to send a request, protect it with a secret, and inspect the agent run."
            docsPath="automations/http-webhooks"
          />

          <form onSubmit={handleSubmit} className="flex flex-col gap-6">
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
                  icon="users"
                  title="No teams"
                  body="Create a team before adding a webhook trigger."
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
              description="Select the agent that should handle inbound webhook requests."
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
                  body="Add an agent to this team before adding a webhook trigger."
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
              title="Shared secret"
              description="Optional. When set, requests must send this secret in the Authorization header. Store it now — it can't be shown again."
            >
              <Input
                value={secret}
                onChange={(event) => setSecret(event.target.value)}
                placeholder="Leave blank for an unauthenticated webhook"
                className="h-9 w-full rounded-md"
              />
            </FormSection>

            <div className="flex justify-end">
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
                  <AppIcon icon="plus" className="h-4 w-4" />
                )}
                Create webhook trigger
              </Button>
            </div>
          </form>
        </div>
      </div>
    </div>
  )
}
