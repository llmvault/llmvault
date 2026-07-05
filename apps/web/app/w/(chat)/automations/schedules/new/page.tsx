"use client"

import { FormEvent, useMemo, useState } from "react"
import { useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import {
  Button,
  Input,
  ListBox,
  Select,
  Spinner,
  TextArea,
  toast,
} from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { AgentSelect } from "@/components/agent-select"
import {
  ChannelSelect,
  useHivyChannels,
} from "@/app/w/(chat)/automations/_channel-select"
import {
  FormSection,
  InlineNotice,
} from "@/app/w/(chat)/automations/_trigger-form-sections"

const DEFAULT_TASK_PROMPT =
  "Describe the recurring task the agent should run each time this schedule fires."

type CadenceMode = "interval" | "cron"

const UNIT_SECONDS: Record<string, number> = {
  minutes: 60,
  hours: 3600,
  days: 86400,
}

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
  const [mode, setMode] = useState<CadenceMode>("interval")
  const [intervalValue, setIntervalValue] = useState("30")
  const [intervalUnit, setIntervalUnit] = useState("minutes")
  const [cron, setCron] = useState("0 9 * * *")

  const activeChannelID = channelID || channels[0]?.id || ""
  const activeAgentID = agents.some((agent) => agent.id === agentID)
    ? agentID
    : (agents[0]?.id ?? "")

  const intervalSeconds =
    Math.round(Number(intervalValue) || 0) * (UNIT_SECONDS[intervalUnit] ?? 60)
  const isLoading = channelsLoading || agentsQuery.isLoading
  const isSaving = createSchedule.isPending
  const cadenceValid =
    mode === "cron" ? Boolean(cron.trim()) : intervalSeconds > 0
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
    if (!cadenceValid) {
      toast.danger(
        mode === "cron" ? "Enter a cron expression" : "Enter an interval"
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
          ...(mode === "cron"
            ? { cron_expression: cron.trim() }
            : { interval_seconds: intervalSeconds }),
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
              <h1 className="text-2xl font-semibold text-foreground">
                Add schedule
              </h1>
              <p className="text-muted-foreground mt-1 text-sm">
                Run an agent on a recurring interval or cron schedule.
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
              description="How often the agent should run. All times are UTC."
            >
              <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
                <ModeSelect value={mode} onChange={setMode} />
                {mode === "interval" ? (
                  <div className="flex flex-1 items-center gap-2">
                    <Input
                      type="number"
                      min={1}
                      value={intervalValue}
                      onChange={(event) => setIntervalValue(event.target.value)}
                      className="h-9 w-24 rounded-md"
                    />
                    <UnitSelect
                      value={intervalUnit}
                      onChange={setIntervalUnit}
                    />
                  </div>
                ) : (
                  <Input
                    value={cron}
                    onChange={(event) => setCron(event.target.value)}
                    placeholder="0 9 * * *"
                    className="h-9 flex-1 rounded-md font-mono"
                  />
                )}
              </div>
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

function ModeSelect({
  value,
  onChange,
}: {
  value: CadenceMode
  onChange: (value: CadenceMode) => void
}) {
  return (
    <Select
      aria-label="Schedule type"
      selectedKey={value}
      onSelectionChange={(key) => {
        if (key !== null) onChange(String(key) as CadenceMode)
      }}
      className="w-full sm:w-40"
    >
      <Select.Trigger className="h-9 w-full justify-between px-3 text-sm">
        <Select.Value />
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover className="w-40 p-1.5">
        <ListBox>
          <ListBox.Item id="interval" textValue="Every interval">
            Every interval
          </ListBox.Item>
          <ListBox.Item id="cron" textValue="Cron expression">
            Cron expression
          </ListBox.Item>
        </ListBox>
      </Select.Popover>
    </Select>
  )
}

function UnitSelect({
  value,
  onChange,
}: {
  value: string
  onChange: (value: string) => void
}) {
  return (
    <Select
      aria-label="Interval unit"
      selectedKey={value}
      onSelectionChange={(key) => {
        if (key !== null) onChange(String(key))
      }}
      className="w-full sm:w-32"
    >
      <Select.Trigger className="h-9 w-full justify-between px-3 text-sm">
        <Select.Value />
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover className="w-32 p-1.5">
        <ListBox>
          <ListBox.Item id="minutes" textValue="Minutes">
            Minutes
          </ListBox.Item>
          <ListBox.Item id="hours" textValue="Hours">
            Hours
          </ListBox.Item>
          <ListBox.Item id="days" textValue="Days">
            Days
          </ListBox.Item>
        </ListBox>
      </Select.Popover>
    </Select>
  )
}
