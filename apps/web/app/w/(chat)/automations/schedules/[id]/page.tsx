"use client"

import { useMemo, useState } from "react"
import { useParams, useRouter } from "next/navigation"
import NextLink from "next/link"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Input, Spinner, Switch, TextArea, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { useIsAdmin } from "@/lib/auth/use-role"
import { useTeamAgents } from "@/lib/api/team-agents"
import { slugify } from "@/app/w/(chat)/_lib/sidebar-data"
import {
  ChannelSelect,
  useHivyChannels,
} from "@/app/w/(chat)/automations/_channel-select"
import {
  ScheduleCadenceFields,
  scheduleToCadenceState,
  type Cadence,
} from "@/app/w/(chat)/automations/_schedule-cadence"
import type { ScheduleItem } from "@/app/w/(chat)/automations/_data"
import {
  FormSection,
  InlineNotice,
} from "@/app/w/(chat)/automations/_trigger-form-sections"
import { TriggerDeleteConfirmModal } from "@/app/w/(chat)/automations/_trigger-delete-confirm-modal"

export default function ScheduleDetailPage() {
  const params = useParams<{ id: string }>()
  const id = params.id
  const router = useRouter()

  const scheduleQuery = $api.useQuery(
    "get",
    "/v1/schedules/{id}",
    { params: { path: { id } } },
    { retry: false }
  )
  const schedule = scheduleQuery.data?.schedule

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

          {scheduleQuery.isLoading ? (
            <div className="flex min-h-56 items-center justify-center">
              <Spinner />
            </div>
          ) : scheduleQuery.isError || !schedule ? (
            <div className="bg-card flex min-h-56 flex-col items-center justify-center rounded-xl px-6 text-center">
              <AppIcon icon="triangle-alert" className="text-muted h-7 w-7" />
              <p className="mt-3 text-sm font-medium text-foreground">
                Schedule not found
              </p>
            </div>
          ) : (
            <ScheduleEditForm key={schedule.id} schedule={schedule} />
          )}
        </div>
      </div>
    </div>
  )
}

function ScheduleEditForm({ schedule }: { schedule: ScheduleItem }) {
  const router = useRouter()
  const queryClient = useQueryClient()
  const id = schedule.id ?? ""

  // Editing an existing schedule (toggle status, save, delete) mutates via
  // PATCH/DELETE /v1/schedules/{id}, which is admin-only on the backend.
  // Creating a schedule is a member action and isn't gated here.
  const isAdmin = useIsAdmin()
  const { channels, isLoading: channelsLoading } = useHivyChannels()
  const updateSchedule = $api.useMutation("patch", "/v1/schedules/{id}")
  const deleteSchedule = $api.useMutation("delete", "/v1/schedules/{id}")

  const [name, setName] = useState(schedule.name ?? "")
  const [channelID, setChannelID] = useState(schedule.channel_id ?? "")
  const [taskPrompt, setTaskPrompt] = useState(schedule.task_prompt ?? "")
  const [cadence, setCadence] = useState<Cadence | null>(null)
  const [deleteOpen, setDeleteOpen] = useState(false)

  const initialCadence = useMemo(
    () => scheduleToCadenceState(schedule),
    [schedule]
  )
  const active = schedule.status === "active"
  const activeChannelID = channelID || channels[0]?.id || ""
  const activeChannel = channels.find((c) => c.id === activeChannelID)

  // The schedule's agent is fixed. When the user retargets it to a channel on a
  // different team the backend rejects the save (409/422), since an agent can
  // only run in its own team's channels. Pre-check the target channel's team
  // agents so we can warn and block Save before the round-trip.
  const { agents: teamAgents, isLoading: teamAgentsLoading } = useTeamAgents(
    activeChannel?.team_id
  )
  const agentOnChannelTeam =
    teamAgentsLoading ||
    !schedule.agent_id ||
    teamAgents.some((agent) => agent.id === schedule.agent_id)

  const channel = channels.find((c) => c.id === schedule.channel_id)
  const lastRunHref =
    schedule.last_run_session_id && channel?.name
      ? `/w/channels/${slugify(channel.name)}/${schedule.last_run_session_id}`
      : null

  const cadenceValid = Boolean(cadence && "body" in cadence)
  const canSave = Boolean(
    isAdmin &&
      !updateSchedule.isPending &&
      name.trim() &&
      activeChannelID &&
      taskPrompt.trim() &&
      cadenceValid &&
      agentOnChannelTeam
  )

  function invalidate() {
    queryClient.invalidateQueries({ queryKey: ["get", "/v1/schedules"] })
    queryClient.invalidateQueries({ queryKey: ["get", "/v1/schedules/{id}"] })
  }

  function handleSave() {
    if (!name.trim() || !taskPrompt.trim() || !activeChannelID) {
      toast.danger("Fill in name, channel and task")
      return
    }
    if (!cadence || !("body" in cadence)) {
      toast.danger(
        cadence && "error" in cadence ? cadence.error : "Set a schedule"
      )
      return
    }
    updateSchedule.mutate(
      {
        params: { path: { id } },
        body: {
          name: name.trim(),
          channel_id: activeChannelID,
          task_prompt: taskPrompt.trim(),
          ...cadence.body,
        },
      },
      {
        onSuccess: () => {
          toast.success("Schedule saved")
          invalidate()
        },
        onError: (error) =>
          toast.danger(extractErrorMessage(error, "Could not save schedule")),
      }
    )
  }

  function toggleActive(next: boolean) {
    updateSchedule.mutate(
      {
        params: { path: { id } },
        body: { status: next ? "active" : "paused" },
      },
      {
        onSuccess: () => {
          toast.success(next ? "Schedule resumed" : "Schedule paused")
          invalidate()
        },
        onError: (error) =>
          toast.danger(extractErrorMessage(error, "Could not update schedule")),
      }
    )
  }

  function confirmDelete() {
    deleteSchedule.mutate(
      { params: { path: { id } } },
      {
        onSuccess: () => {
          setDeleteOpen(false)
          toast.success("Schedule deleted")
          invalidate()
          router.push("/w/automations?tab=schedules")
        },
        onError: (error) =>
          toast.danger(extractErrorMessage(error, "Could not delete schedule")),
      }
    )
  }

  return (
    <>
      <header className="flex items-center gap-3">
        <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl bg-violet-500 text-white">
          <AppIcon icon="calendar" className="h-6 w-6" />
        </div>
        <div className="min-w-0">
          <h1 className="truncate text-lg font-semibold text-foreground">
            {schedule.name || schedule.agent_name || "Schedule"}
          </h1>
          <p className="text-muted-foreground mt-1 text-sm">
            Edit this recurring schedule.
          </p>
        </div>
      </header>

      <div className="flex gap-2.5 rounded-xl border border-sky-500/25 bg-sky-500/[0.06] px-3.5 py-3">
        <AppIcon icon="clock" className="mt-0.5 h-4 w-4 shrink-0 text-sky-500" />
        <div className="flex flex-col gap-1 text-sm leading-5">
          <span className="font-medium text-foreground">
            {active
              ? schedule.next_run_at
                ? `Next run ${formatDateTime(schedule.next_run_at)}`
                : "Active"
              : "Paused — not scheduled to run"}
          </span>
          <span className="text-muted-foreground">
            {schedule.last_run_at ? (
              <>
                Last run {formatDateTime(schedule.last_run_at)}
                {schedule.last_status ? ` · ${schedule.last_status}` : ""}
                {lastRunHref ? (
                  <>
                    {" · "}
                    <NextLink
                      href={lastRunHref}
                      className="font-medium text-sky-600 underline underline-offset-2 hover:text-sky-500"
                    >
                      View session
                    </NextLink>
                  </>
                ) : null}
              </>
            ) : (
              "Hasn't run yet."
            )}
          </span>
        </div>
      </div>

      <form
        onSubmit={(event) => {
          event.preventDefault()
          handleSave()
        }}
        className="flex flex-col gap-6"
      >
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
              body="No channels available."
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
          description="The agent that runs on this schedule. Create a new schedule to use a different agent."
        >
          <div className="bg-field flex h-9 items-center gap-2 rounded-md border border-border px-3 text-sm">
            <AppIcon icon="bot" className="text-muted h-4 w-4 shrink-0" />
            <span className="truncate text-foreground">
              {schedule.agent_name || "Agent"}
            </span>
          </div>
          {!agentOnChannelTeam ? (
            <p className="mt-2 flex items-start gap-1.5 text-sm leading-5 text-warning">
              <AppIcon
                icon="triangle-alert"
                className="mt-0.5 h-4 w-4 shrink-0"
              />
              <span>
                {schedule.agent_name || "This agent"} isn&apos;t on the selected
                channel&apos;s team. Pick a channel on the agent&apos;s team
                instead.
              </span>
            </p>
          ) : null}
        </FormSection>

        <FormSection
          title="Repeat"
          description="Pick when this runs in your local time — schedules execute in UTC and we convert for you."
        >
          <ScheduleCadenceFields
            initial={initialCadence}
            onChange={setCadence}
          />
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

        <FormSection
          title="Status"
          description={
            active
              ? "The agent runs on this schedule."
              : "The schedule is paused and won't run."
          }
        >
          <div className="flex items-center justify-between gap-4 rounded-xl border border-border px-3 py-2.5">
            <span className="text-sm font-medium text-foreground">
              {active ? "Active" : "Paused"}
            </span>
            <Switch
              aria-label="Activate schedule"
              isSelected={active}
              isDisabled={updateSchedule.isPending || !isAdmin}
              onChange={toggleActive}
              className="shrink-0"
            >
              <Switch.Control>
                <Switch.Thumb />
              </Switch.Control>
            </Switch>
          </div>
        </FormSection>

        {!isAdmin ? (
          <p className="text-sm text-muted-foreground">
            Only workspace admins can edit or delete automations.
          </p>
        ) : null}

        <div className="flex items-center justify-between gap-3">
          <Button
            type="button"
            variant="secondary"
            size="sm"
            className="text-danger"
            isDisabled={deleteSchedule.isPending || !isAdmin}
            onPress={() => setDeleteOpen(true)}
          >
            {deleteSchedule.isPending ? (
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
            {updateSchedule.isPending ? (
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
        pending={deleteSchedule.isPending}
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
