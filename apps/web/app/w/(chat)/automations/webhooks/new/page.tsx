"use client"

import { FormEvent, useState } from "react"
import { useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Input, Spinner, TextArea, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import {
  resolveScopedAgentID,
  useChannelScopedAgents,
} from "@/lib/api/channel-scoped-agents"
import { AgentSelect } from "@/components/agent-select"
import {
  ChannelSelect,
  useHivyChannels,
} from "@/app/w/(chat)/automations/_channel-select"
import {
  FormSection,
  InlineNotice,
} from "@/app/w/(chat)/automations/_trigger-form-sections"

const DEFAULT_INSTRUCTIONS =
  "You were triggered by an inbound webhook request. Read the request payload, decide what action it calls for, and carry it out. If there isn't enough context to act confidently, summarise what you received instead of guessing."

export default function NewWebhookTriggerPage() {
  const router = useRouter()
  const queryClient = useQueryClient()

  const { channels, isLoading: channelsLoading } = useHivyChannels()
  const createTrigger = $api.useMutation("post", "/v1/triggers")

  const [name, setName] = useState("")
  const [channelID, setChannelID] = useState("")
  const [agentID, setAgentID] = useState("")
  const [instructions, setInstructions] = useState(DEFAULT_INSTRUCTIONS)
  const [secret, setSecret] = useState("")

  const activeChannelID = channelID || channels[0]?.id || ""
  const activeChannel = channels.find((c) => c.id === activeChannelID)
  const {
    agents,
    isLoading: agentsLoading,
  } = useChannelScopedAgents(activeChannelID)
  const activeAgentID = resolveScopedAgentID(
    agents,
    agentID,
    activeChannel?.default_agent_id
  )

  const isSaving = createTrigger.isPending
  const canSubmit = Boolean(
    !isSaving &&
      name.trim() &&
      activeChannelID &&
      activeAgentID &&
      instructions.trim()
  )

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!name.trim()) {
      toast.danger("Name is required")
      return
    }
    if (!activeChannelID) {
      toast.danger("Select a channel")
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
          channel_id: activeChannelID,
          agent_id: activeAgentID,
          instructions: trimmed,
          secret_key: secret.trim() || undefined,
        },
      },
      {
        onSuccess: () => {
          toast.success("Webhook trigger created")
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/triggers"] })
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
            className="text-muted-foreground hover:text-foreground flex w-fit items-center gap-1.5 text-sm transition-colors"
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
              title="Channel"
              description="The channel this webhook's agent runs in. You can only pick channels you have access to."
            >
              {channelsLoading ? (
                <div className="h-9 animate-pulse rounded-md bg-default" />
              ) : channels.length === 0 ? (
                <InlineNotice
                  icon="hash"
                  title="No channels"
                  body="Create a channel before adding a webhook trigger."
                />
              ) : (
                <ChannelSelect
                  channels={channels}
                  value={activeChannelID}
                  onChange={setChannelID}
                />
              )}
            </FormSection>

            <FormSection
              title="Agent"
              description="Select the agent that should handle inbound webhook requests. Only agents assigned to the chosen channel can run here."
            >
              {!activeChannelID ? (
                <InlineNotice
                  icon="bot"
                  title="Select a channel first"
                  body="Agents are scoped to the channel this webhook runs in."
                />
              ) : agents.length === 0 && !agentsLoading ? (
                <InlineNotice
                  icon="bot"
                  title="No agents in this channel"
                  body="Assign an agent to the selected channel before adding a webhook trigger."
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
