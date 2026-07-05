"use client"

import { useState } from "react"
import { useParams, useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Spinner, Switch, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { TriggerDeleteConfirmModal } from "@/app/w/(chat)/automations/_trigger-delete-confirm-modal"

export default function WebhookTriggerDetailPage() {
  const params = useParams<{ id: string }>()
  const id = params.id
  const router = useRouter()
  const queryClient = useQueryClient()

  const triggerQuery = $api.useQuery(
    "get",
    "/v1/triggers/{id}",
    { params: { path: { id } } },
    { retry: false }
  )
  const updateTrigger = $api.useMutation("patch", "/v1/triggers/{id}")
  const deleteTrigger = $api.useMutation("delete", "/v1/triggers/{id}")
  const [deleteOpen, setDeleteOpen] = useState(false)

  const trigger = triggerQuery.data?.trigger

  function invalidate() {
    queryClient.invalidateQueries({ queryKey: ["get", "/v1/triggers"] })
    queryClient.invalidateQueries({ queryKey: ["get", "/v1/triggers/{id}"] })
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
    if (!trigger?.webhook_url) return
    try {
      await navigator.clipboard.writeText(trigger.webhook_url)
      toast.success("Webhook URL copied")
    } catch {
      toast.danger("Could not copy URL")
    }
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

          {triggerQuery.isLoading ? (
            <div className="flex min-h-56 items-center justify-center">
              <Spinner />
            </div>
          ) : triggerQuery.isError || !trigger ? (
            <div className="bg-card flex min-h-56 flex-col items-center justify-center rounded-xl px-6 text-center">
              <AppIcon icon="triangle-alert" className="text-muted h-7 w-7" />
              <p className="mt-3 text-sm font-medium text-foreground">
                Webhook trigger not found
              </p>
            </div>
          ) : (
            <>
              <header className="flex items-center gap-3">
                <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl bg-sky-500 text-white">
                  <AppIcon icon="globe" className="h-6 w-6" />
                </div>
                <div className="min-w-0">
                  <h1 className="truncate text-2xl font-semibold text-foreground">
                    {trigger.name || trigger.agent_name || "Webhook trigger"}
                  </h1>
                  <p className="text-muted-foreground mt-1 text-sm">
                    Runs when the webhook URL below receives a request.
                  </p>
                </div>
              </header>

              <section className="flex flex-col gap-3">
                <h2 className="text-sm font-semibold text-foreground">
                  Webhook URL
                </h2>
                <div className="flex items-center gap-2 rounded-xl border border-border bg-card px-3 py-2.5">
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

              <section className="grid grid-cols-1 gap-3 sm:grid-cols-3">
                <MetaCard
                  icon="hash"
                  label="Channel"
                  value={trigger.channel_name || "—"}
                />
                <MetaCard
                  icon="bot"
                  label="Agent"
                  value={trigger.agent_name || "—"}
                />
                <MetaCard
                  icon={trigger.secret_set ? "shield-check" : "circle-alert"}
                  label="Auth"
                  value={trigger.secret_set ? "Shared secret" : "Public"}
                />
              </section>

              {trigger.instructions ? (
                <section className="flex flex-col gap-3">
                  <h2 className="text-sm font-semibold text-foreground">
                    Instructions
                  </h2>
                  <p className="rounded-xl border border-border px-3 py-2.5 text-sm leading-5 whitespace-pre-wrap text-foreground">
                    {trigger.instructions}
                  </p>
                </section>
              ) : null}

              <section className="flex flex-col gap-3">
                <h2 className="text-sm font-semibold text-foreground">Status</h2>
                <div className="flex items-center justify-between gap-4 rounded-xl border border-border px-3 py-2.5">
                  <div className="flex min-w-0 flex-col gap-0.5">
                    <span className="text-sm font-medium text-foreground">
                      {trigger.enabled ? "Enabled" : "Disabled"}
                    </span>
                    <span className="text-muted-foreground text-sm leading-5">
                      {trigger.enabled
                        ? "Incoming requests will run this agent."
                        : "Incoming requests are ignored."}
                    </span>
                  </div>
                  <Switch
                    aria-label="Enable webhook"
                    isSelected={trigger.enabled ?? true}
                    isDisabled={updateTrigger.isPending}
                    onChange={toggleEnabled}
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
                  isDisabled={deleteTrigger.isPending}
                  onPress={() => setDeleteOpen(true)}
                >
                  {deleteTrigger.isPending ? (
                    <Spinner color="current" size="sm" />
                  ) : (
                    <AppIcon icon="trash-2" className="h-4 w-4" />
                  )}
                  Delete webhook
                </Button>
              </div>

              <TriggerDeleteConfirmModal
                open={deleteOpen}
                pending={deleteTrigger.isPending}
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
