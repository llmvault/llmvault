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
  async function copyAddress() {
    const email = inbox?.email?.trim()
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
      className="flex flex-col gap-4"
      aria-labelledby="agent-inbox-heading"
    >
      <div>
        <h2
          id="agent-inbox-heading"
          className="text-sm font-semibold text-foreground"
        >
          Agent inbox
        </h2>
        <p className="text-muted-foreground mt-1 max-w-2xl text-sm leading-5">
          Send email to this address to start a new session with this agent.
        </p>
      </div>

      {isLoading ? (
        <InboxSkeleton />
      ) : isError ? (
        <div className="bg-card flex min-h-48 flex-col items-center justify-center rounded-xl px-6 text-center">
          <AppIcon
            icon="triangle-alert"
            className="text-muted-foreground h-7 w-7"
          />
          <p className="mt-3 text-sm font-medium text-foreground">
            Could not load inbox
          </p>
          <p className="text-muted-foreground mt-1 max-w-sm text-sm">
            Try again to load this agent&apos;s inbox.
          </p>
          {onRetry ? (
            <Button
              type="button"
              variant="secondary"
              size="sm"
              className="mt-4"
              onPress={onRetry}
            >
              Try again
            </Button>
          ) : null}
        </div>
      ) : !inbox?.available ? (
        <div className="bg-card flex min-h-56 flex-col items-center justify-center rounded-xl px-6 text-center">
          <AppIcon icon="mail" className="text-muted-foreground h-7 w-7" />
          <p className="mt-3 text-sm font-medium text-foreground">
            No inbox yet
          </p>
          <p className="text-muted-foreground mt-1 max-w-sm text-sm leading-5">
            Add a dedicated email address so people can start sessions with this
            agent by email.
          </p>
          <Button
            type="button"
            variant="primary"
            size="sm"
            className="mt-4"
            isPending={isProvisioning}
            isDisabled={isProvisioning}
            onPress={onProvision}
          >
            Add inbox
          </Button>
        </div>
      ) : (
        <div className="bg-card rounded-xl p-5">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="min-w-0">
              <p className="text-muted-foreground text-xs font-medium">
                Email address
              </p>
              <p className="mt-1 truncate text-sm font-medium text-foreground">
                {inbox.email}
              </p>
            </div>
            <Button
              type="button"
              variant="secondary"
              size="sm"
              onPress={() => void copyAddress()}
            >
              <AppIcon icon="copy" className="h-4 w-4" />
              Copy
            </Button>
          </div>
          <div className="mt-5 border-t border-border pt-4">
            <p className="text-muted-foreground text-xs font-medium">
              Inbox messages
            </p>
            <p className="mt-1 text-2xl font-semibold text-foreground">
              {inbox.message_count ?? 0}
            </p>
          </div>
        </div>
      )}
    </section>
  )
}

function InboxSkeleton() {
  return (
    <div className="bg-card rounded-xl p-5">
      <Skeleton className="h-4 w-24 rounded-md" />
      <Skeleton className="mt-3 h-5 w-72 max-w-full rounded-md" />
      <Skeleton className="mt-6 h-14 w-full rounded-md" />
    </div>
  )
}
