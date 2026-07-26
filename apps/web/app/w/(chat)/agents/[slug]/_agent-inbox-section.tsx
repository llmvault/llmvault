"use client"

import { Button, Skeleton, toast } from "@heroui/react"
import { useQueryClient } from "@tanstack/react-query"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { queryKeys } from "@/lib/api/query-keys"
import type { components } from "@/lib/api/schema"

type AgentInboxResponse = components["schemas"]["agentInboxResponse"]

export function AgentInboxSection({ agentId }: { agentId: string }) {
  const queryClient = useQueryClient()
  const inboxQuery = $api.useQuery("get", "/v1/agents/{id}/inbox", {
    params: { path: { id: agentId } },
  })
  const provisionInbox = $api.useMutation("post", "/v1/agents/{id}/inbox")

  async function provision() {
    try {
      await provisionInbox.mutateAsync({
        params: { path: { id: agentId } },
      })
      await queryClient.invalidateQueries({
        queryKey: queryKeys.agentInbox(agentId),
      })
      toast.success("Agent inbox added")
    } catch (error) {
      toast.danger(extractErrorMessage(error, "Could not add agent inbox"))
    }
  }

  return (
    <AgentInboxView
      inbox={inboxQuery.data}
      isLoading={inboxQuery.isLoading}
      isError={inboxQuery.isError}
      isProvisioning={provisionInbox.isPending}
      onProvision={() => void provision()}
      onRetry={() => void inboxQuery.refetch()}
    />
  )
}

export function AgentInboxView({
  inbox,
  isLoading = false,
  isError = false,
  isProvisioning = false,
  onProvision,
  onRetry,
}: {
  inbox?: AgentInboxResponse
  isLoading?: boolean
  isError?: boolean
  isProvisioning?: boolean
  onProvision: () => void
  onRetry?: () => void
}) {
  const email = inbox?.email?.trim() ?? ""
  const messageCount = inbox?.message_count ?? 0

  async function copyAddress() {
    if (!email) return
    try {
      await navigator.clipboard.writeText(email)
      toast.success("Inbox address copied")
    } catch {
      toast.danger("Could not copy inbox address")
    }
  }

  return (
    <section
      className="flex flex-col gap-5"
      aria-labelledby="agent-inbox-heading"
    >
      <div>
        <h2
          id="agent-inbox-heading"
          className="text-base font-semibold text-foreground"
        >
          Start a session by email
        </h2>
        <p className="text-muted-foreground mt-1 max-w-2xl text-sm leading-5">
          Messages sent to this agent&apos;s dedicated address become new
          sessions.
        </p>
      </div>

      {isLoading ? (
        <InboxSkeleton />
      ) : isError ? (
        <div className="flex flex-col gap-4 rounded-2xl border border-border bg-surface p-5 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-start gap-3">
            <span className="text-muted-foreground flex size-9 shrink-0 items-center justify-center rounded-lg bg-default">
              <AppIcon icon="triangle-alert" className="size-4" />
            </span>
            <div>
              <p className="text-sm font-medium text-foreground">
                Could not load the inbox
              </p>
              <p className="text-muted-foreground mt-0.5 text-sm">
                Check the connection and try again.
              </p>
            </div>
          </div>
          {onRetry ? (
            <Button
              type="button"
              variant="secondary"
              size="sm"
              className="w-full shrink-0 sm:w-auto"
              onPress={onRetry}
            >
              Try again
            </Button>
          ) : null}
        </div>
      ) : !inbox?.available ? (
        <div className="flex flex-col gap-5 rounded-2xl border border-border bg-surface p-5 sm:flex-row sm:items-center sm:justify-between sm:p-6">
          <div className="flex items-start gap-4">
            <span className="text-muted-foreground flex size-10 shrink-0 items-center justify-center rounded-xl bg-default">
              <AppIcon icon="mail" className="size-5" />
            </span>
            <div>
              <p className="text-sm font-medium text-foreground">
                Give this agent an email address
              </p>
              <p className="text-muted-foreground mt-1 max-w-md text-sm leading-5">
                Create a dedicated inbox so teammates can start sessions by
                sending an email.
              </p>
            </div>
          </div>
          <Button
            type="button"
            variant="primary"
            size="sm"
            className="w-full shrink-0 sm:w-auto"
            isPending={isProvisioning}
            isDisabled={isProvisioning}
            onPress={onProvision}
          >
            Add inbox
          </Button>
        </div>
      ) : (
        <div className="overflow-hidden rounded-2xl border border-border bg-surface">
          <div className="p-5 sm:p-6">
            <div className="flex items-center justify-between gap-4">
              <p className="text-muted-foreground text-xs font-medium">
                Agent email address
              </p>
              <span className="text-muted-foreground flex shrink-0 items-center gap-2 text-xs font-medium">
                <span
                  className="size-1.5 rounded-full bg-success"
                  aria-hidden="true"
                />
                Inbox active
              </span>
            </div>

            <div className="mt-3 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <p className="text-base leading-6 font-medium break-all text-foreground">
                {email || "Address unavailable"}
              </p>
              <Button
                type="button"
                variant="secondary"
                size="sm"
                className="w-full shrink-0 sm:w-auto"
                isDisabled={!email}
                onPress={() => void copyAddress()}
              >
                <AppIcon icon="copy" className="size-4" />
                Copy address
              </Button>
            </div>
          </div>

          <div className="text-muted-foreground flex items-center gap-2 border-t border-border bg-default/40 px-5 py-3 text-sm sm:px-6">
            <AppIcon
              icon="messages-square"
              className="size-4 shrink-0"
              aria-hidden="true"
            />
            <p>{messageCountLabel(messageCount)}</p>
          </div>
        </div>
      )}
    </section>
  )
}

function InboxSkeleton() {
  return (
    <div className="overflow-hidden rounded-2xl border border-border bg-surface">
      <div className="p-5 sm:p-6">
        <div className="flex items-center justify-between">
          <Skeleton className="h-3 w-28 rounded" />
          <Skeleton className="h-3 w-20 rounded" />
        </div>
        <div className="mt-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <Skeleton className="h-5 w-72 max-w-full rounded" />
          <Skeleton className="h-8 w-full rounded-lg sm:w-32" />
        </div>
      </div>
      <div className="border-t border-border bg-default/40 px-5 py-3 sm:px-6">
        <Skeleton className="h-4 w-40 rounded" />
      </div>
    </div>
  )
}

function messageCountLabel(count: number) {
  if (count === 0) return "No messages received yet"
  return `${count} ${count === 1 ? "message" : "messages"} received`
}
