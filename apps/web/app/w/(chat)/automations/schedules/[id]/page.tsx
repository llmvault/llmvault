"use client"

import { useState } from "react"
import { useParams, useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Spinner, Switch, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { scheduleCadenceLabel } from "@/app/w/(chat)/automations/_data"
import { TriggerDeleteConfirmModal } from "@/app/w/(chat)/automations/_trigger-delete-confirm-modal"

export default function ScheduleDetailPage() {
  const params = useParams<{ id: string }>()
  const id = params.id
  const router = useRouter()
  const queryClient = useQueryClient()

  const scheduleQuery = $api.useQuery(
    "get",
    "/v1/schedules/{id}",
    { params: { path: { id } } },
    { retry: false }
  )
  const updateSchedule = $api.useMutation("patch", "/v1/schedules/{id}")
  const deleteSchedule = $api.useMutation("delete", "/v1/schedules/{id}")
  const [deleteOpen, setDeleteOpen] = useState(false)

  const schedule = scheduleQuery.data?.schedule

  function invalidate() {
    queryClient.invalidateQueries({ queryKey: ["get", "/v1/schedules"] })
    queryClient.invalidateQueries({ queryKey: ["get", "/v1/schedules/{id}"] })
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

  const active = schedule?.status === "active"

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
            <>
              <header className="flex items-center gap-3">
                <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl bg-violet-500 text-white">
                  <AppIcon icon="calendar" className="h-6 w-6" />
                </div>
                <div className="min-w-0">
                  <h1 className="truncate text-2xl font-semibold text-foreground">
                    {schedule.name || schedule.agent_name || "Schedule"}
                  </h1>
                  <p className="text-muted-foreground mt-1 text-sm">
                    {scheduleCadenceLabel(schedule)}
                  </p>
                </div>
              </header>

              <section className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <MetaCard
                  icon="bot"
                  label="Agent"
                  value={schedule.agent_name || "—"}
                />
                <MetaCard
                  icon="clock"
                  label="Next run"
                  value={formatDateTime(schedule.next_run_at)}
                />
                <MetaCard
                  icon="clock"
                  label="Last run"
                  value={
                    schedule.last_run_at
                      ? `${formatDateTime(schedule.last_run_at)}${schedule.last_status ? ` · ${schedule.last_status}` : ""}`
                      : "Never"
                  }
                />
                <MetaCard
                  icon="rotate-cw"
                  label="Repeat"
                  value={
                    schedule.repeat_count
                      ? `${schedule.repeat_completed ?? 0} / ${schedule.repeat_count} runs`
                      : "Until deleted"
                  }
                />
              </section>

              {schedule.task_prompt ? (
                <section className="flex flex-col gap-3">
                  <h2 className="text-sm font-semibold text-foreground">Task</h2>
                  <p className="rounded-xl border border-border px-3 py-2.5 text-sm leading-5 whitespace-pre-wrap text-foreground">
                    {schedule.task_prompt}
                  </p>
                </section>
              ) : null}

              <section className="flex flex-col gap-3">
                <h2 className="text-sm font-semibold text-foreground">Status</h2>
                <div className="flex items-center justify-between gap-4 rounded-xl border border-border px-3 py-2.5">
                  <div className="flex min-w-0 flex-col gap-0.5">
                    <span className="text-sm font-medium text-foreground">
                      {active ? "Active" : "Paused"}
                    </span>
                    <span className="text-muted-foreground text-sm leading-5">
                      {active
                        ? "The agent runs on this schedule."
                        : "The schedule is paused and won't run."}
                    </span>
                  </div>
                  <Switch
                    aria-label="Activate schedule"
                    isSelected={active}
                    isDisabled={updateSchedule.isPending}
                    onChange={toggleActive}
                    className="shrink-0"
                  >
                    <Switch.Control>
                      <Switch.Thumb />
                    </Switch.Control>
                  </Switch>
                </div>
              </section>

              <div className="flex justify-end">
                <Button
                  type="button"
                  variant="secondary"
                  size="sm"
                  className="text-danger"
                  isDisabled={deleteSchedule.isPending}
                  onPress={() => setDeleteOpen(true)}
                >
                  {deleteSchedule.isPending ? (
                    <Spinner color="current" size="sm" />
                  ) : (
                    <AppIcon icon="trash-2" className="h-4 w-4" />
                  )}
                  Delete schedule
                </Button>
              </div>

              <TriggerDeleteConfirmModal
                open={deleteOpen}
                pending={deleteSchedule.isPending}
                onOpenChange={setDeleteOpen}
                onConfirm={confirmDelete}
              />
            </>
          )}
        </div>
      </div>
    </div>
  )
}

function MetaCard({
  icon,
  label,
  value,
}: {
  icon: string
  label: string
  value: string
}) {
  return (
    <div className="flex flex-col gap-1 rounded-xl border border-border px-3 py-2.5">
      <span className="text-muted-foreground flex items-center gap-1.5 text-xs">
        <AppIcon icon={icon} className="h-3.5 w-3.5" />
        {label}
      </span>
      <span className="truncate text-sm font-medium text-foreground">
        {value}
      </span>
    </div>
  )
}

function formatDateTime(value?: string): string {
  if (!value) return "—"
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return "—"
  return date.toLocaleString()
}
