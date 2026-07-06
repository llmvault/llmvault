"use client"

import { FormEvent, useMemo, useState } from "react"
import { useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Input, Spinner, TextArea, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { AgentSelect } from "@/components/agent-select"
import {
  ChannelSelect,
  useHivyChannels,
} from "@/app/w/(chat)/automations/_channel-select"
import {
  ScheduleCadenceFields,
  type Cadence,
} from "@/app/w/(chat)/automations/_schedule-cadence"
import {
  FormSection,
  InlineNotice,
} from "@/app/w/(chat)/automations/_trigger-form-sections"

const DEFAULT_TASK_PROMPT =
  "Describe the recurring task the agent should run each time this schedule fires."

export default function NewSchedulePage() {
  const router = useRouter()
  const queryClient = useQueryClient()

  const { channels, isLoading: channelsLoading } = useHivyChannels()
  const agentsQuery = $api.useQuery("get", "/v1/agents", {
    params: { query: { status: "active", limit: 100 } },
  })
  const createSchedule = $api.useMutation("post", "/v1/schedules")

  const agents = useMemo(
    () => (agentsQuery.data?.data ?? []).filter((agent) => agent.id),
    [agentsQuery.data?.data]
  )

  const [name, setName] = useState("")
  const [channelID, setChannelID] = useState("")
  const [agentID, setAgentID] = useState("")
  const [taskPrompt, setTaskPrompt] = useState(DEFAULT_TASK_PROMPT)
  const [cadence, setCadence] = useState<Cadence | null>(null)

  const activeChannelID = channelID || channels[0]?.id || ""
  const activeAgentID = agents.some((agent) => agent.id === agentID)
    ? agentID
    : (agents[0]?.id ?? "")

  const isLoading = channelsLoading || agentsQuery.isLoading
  const isSaving = createSchedule.isPending
  const cadenceValid = Boolean(cadence && "body" in cadence)
  const canSubmit = Boolean(
    !isSaving &&
      name.trim() &&
      activeChannelID &&
      activeAgentID &&
      taskPrompt.trim() &&
      cadenceValid
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
    if (!taskPrompt.trim()) {
      toast.danger("Task prompt is required")
      return
    }
    if (!cadence || !("body" in cadence)) {
      toast.danger(
        cadence && "error" in cadence ? cadence.error : "Set a schedule"
      )
      return
    }
    createSchedule.mutate(
      {
        body: {
          name: name.trim(),
          agent_id: activeAgentID,
          channel_id: activeChannelID,
          task_prompt: taskPrompt.trim(),
          ...cadence.body,
        },
      },
      {
        onSuccess: () => {
          toast.success("Schedule created")
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/schedules"] })
          router.push("/w/automations?tab=schedules")
        },
        onError: (error) =>
          toast.danger(extractErrorMessage(error, "Could not create schedule")),
      }
    )
  }

  return (
    <div className="h-full overflow-y-auto bg-background text-foreground">
      <div className="mx-auto w-full max-w-2xl px-6 py-12">
        <div className="flex flex-col gap-8">
          <button
            type="button"
            onClick={() => router.push("/w/automations?tab=schedules")}
            className="text-muted-foreground hover:text-foreground flex w-fit items-center gap-1.5 text-sm transition-colors"
          >
            <AppIcon icon="arrow-left" className="h-4 w-4" />
            Schedules
          </button>

          <header className="flex items-center gap-3">
            <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl bg-violet-500 text-white">
              <AppIcon icon="calendar" className="h-6 w-6" />
            </div>
            <div>
              <h1 className="text-lg font-semibold text-foreground">
                Add schedule
              </h1>
              <p className="text-muted-foreground mt-1 text-sm">
                Run an agent on a recurring schedule.
              </p>
            </div>
          </header>

          <form onSubmit={handleSubmit} className="flex flex-col gap-6">
            <FormSection
              title="Name"
              description="A label for this schedule, shown in your automations list."
            >
              <Input
                value={name}
                onChange={(event) => setName(event.target.value)}
                placeholder="Name this schedule"
                className="h-9 w-full rounded-md"
              />
            </FormSection>

            <FormSection
              title="Channel"
              description="The channel this schedule's agent runs in. You can only pick channels you have access to."
            >
              {channelsLoading ? (
                <div className="h-9 animate-pulse rounded-md bg-default" />
              ) : channels.length === 0 ? (
                <InlineNotice
                  icon="hash"
                  title="No channels"
                  body="Create a channel before adding a schedule."
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
              description="Select the agent that should run on this schedule."
            >
              {agents.length === 0 && !isLoading ? (
                <InlineNotice
                  icon="bot"
                  title="No active agents"
                  body="Create or activate an agent before adding a schedule."
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

            <FormSection
              title="Repeat"
              description="Pick when this runs in your local time — schedules execute in UTC and we convert for you."
            >
              <ScheduleCadenceFields onChange={setCadence} />
            </FormSection>

            <FormSection
              title="Task"
              description="What the agent should do each time the schedule fires."
            >
              <TextArea
                value={taskPrompt}
                onChange={(event) => setTaskPrompt(event.target.value)}
                rows={6}
                fullWidth
                className="min-h-36 resize-y leading-5"
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
                Create schedule
              </Button>
            </div>
          </form>
        </div>
      </div>
    </div>
  )
}
