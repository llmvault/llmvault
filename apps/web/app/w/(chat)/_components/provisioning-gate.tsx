"use client"

import { useEffect, useMemo, useState } from "react"
import { Button, Spinner, toast } from "@heroui/react"
import { Icon } from "@iconify/react"
import { $api } from "@/lib/api/hooks"
import { useAuth } from "@/lib/auth/auth-context"
import { extractErrorMessage } from "@/lib/api/error"
import type { components } from "@/lib/api/schema"
import {
  resolveWorkspaceProvisioningState,
  type WorkspaceAgent,
} from "@/app/w/(chat)/_lib/provisioning"

type OrgResponse = components["schemas"]["orgResponse"]

const POLL_INTERVAL_MS = 3000

export function WorkspaceProvisioningGate({
  children,
}: {
  children: React.ReactNode
}) {
  const { activeOrg, isLoading: authLoading } = useAuth()
  const [retryError, setRetryError] = useState<string | null>(null)

  const orgQuery = $api.useQuery(
    "get",
    "/v1/orgs/current",
    {},
    {
      enabled: Boolean(activeOrg?.id),
      retry: false,
    }
  )

  const agentsQuery = $api.useQuery(
    "get",
    "/v1/agents",
    {
      params: {
        query: {
          limit: 50,
        },
      },
    },
    {
      enabled: Boolean(activeOrg?.id),
      retry: false,
    }
  )

  const agents = useMemo(
    () => (agentsQuery.data?.data ?? []) as WorkspaceAgent[],
    [agentsQuery.data?.data]
  )

  const state = resolveWorkspaceProvisioningState({
    authLoading,
    orgLoading: orgQuery.isLoading,
    agentsLoading: agentsQuery.isLoading,
    orgError: orgQuery.isError,
    agentsError: agentsQuery.isError,
    agents,
  })

  useEffect(() => {
    if (!state.shouldPoll) return
    const interval = window.setInterval(() => {
      void agentsQuery.refetch()
    }, POLL_INTERVAL_MS)
    return () => window.clearInterval(interval)
  }, [agentsQuery, state.shouldPoll])

  const syncAgent = $api.useMutation("post", "/v1/agents/{id}/sync", {
    onSuccess: () => {
      setRetryError(null)
      toast.success("Workspace setup restarted")
      void agentsQuery.refetch()
    },
    onError: (error) => {
      const message = extractErrorMessage(
        error,
        "Could not retry workspace setup"
      )
      setRetryError(message)
      toast.danger(message)
    },
  })

  const canManageSetup =
    activeOrg?.role === "owner" ||
    activeOrg?.role === "admin" ||
    activeOrg?.role === undefined
  const hasRetryTarget = Boolean(activeOrg?.id && state.defaultAgent?.id)
  const canRetry = hasRetryTarget && canManageSetup
  const retryHint = !hasRetryTarget
    ? "Retry will be available when the Hivy agent record is ready."
    : "Ask a workspace owner or admin to retry sandbox setup."

  if (state.phase === "ready") {
    return children
  }

  const org = (orgQuery.data as OrgResponse | undefined) ?? null
  const orgName = org?.name ?? activeOrg?.name ?? "your workspace"

  return (
    <WorkspaceProvisioningScreen
      phase={state.phase}
      orgName={orgName}
      status={state.sandboxStatus}
      message={state.message}
      error={retryError ?? state.defaultAgent?.sandbox?.error_message}
      canRetry={canRetry}
      retryHint={retryHint}
      retrying={syncAgent.isPending}
      onRetry={() => {
        if (!state.defaultAgent?.id) return
        syncAgent.mutate({
          params: {
            path: {
              id: state.defaultAgent.id,
            },
          },
        })
      }}
    />
  )
}

function WorkspaceProvisioningScreen({
  phase,
  orgName,
  status,
  message,
  error,
  canRetry,
  retryHint,
  retrying,
  onRetry,
}: {
  phase: "loading" | "provisioning" | "retryable"
  orgName: string
  status?: string
  message: string
  error?: string | null
  canRetry: boolean
  retryHint: string
  retrying: boolean
  onRetry: () => void
}) {
  const loading = phase === "loading" || phase === "provisioning" || retrying
  const title =
    phase === "retryable"
      ? "Workspace setup needs attention"
      : "Setting up your workspace"
  const description =
    phase === "retryable"
      ? "Hivy could not confirm that the managed agent sandbox is ready."
      : `Hivy is preparing the managed agent sandbox for ${orgName}.`

  return (
    <div className="bg-surface flex h-screen w-screen items-center justify-center px-6 text-foreground">
      <div className="flex w-full max-w-md flex-col items-center text-center">
        <div className="mb-6 flex h-12 w-12 items-center justify-center rounded-2xl border border-border bg-background">
          {loading ? (
            <Spinner size="sm" />
          ) : (
            <Icon icon="lucide:triangle-alert" className="h-5 w-5 text-muted" />
          )}
        </div>

        <h1 className="text-xl font-semibold">{title}</h1>
        <p className="mt-2 max-w-sm text-sm leading-6 text-muted">
          {description}
        </p>

        <div className="mt-5 flex flex-col items-center gap-1 text-sm">
          <span className="text-foreground">{message}</span>
          {status ? (
            <span className="text-muted">Sandbox status: {status}</span>
          ) : null}
          {error ? <span className="text-danger max-w-sm">{error}</span> : null}
        </div>

        <div className="mt-7 flex items-center gap-2">
          <Button
            variant="primary"
            isDisabled={!canRetry || retrying}
            onPress={onRetry}
          >
            {retrying ? <Spinner color="current" size="sm" /> : null}
            Retry setup
          </Button>
          <Button
            variant="tertiary"
            isDisabled={retrying}
            onPress={() => window.location.reload()}
          >
            Refresh
          </Button>
        </div>

        {!canRetry ? (
          <p className="mt-4 max-w-sm text-xs leading-5 text-muted">
            {retryHint}
          </p>
        ) : null}
      </div>
    </div>
  )
}
