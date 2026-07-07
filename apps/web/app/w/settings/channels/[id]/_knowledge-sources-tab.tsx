"use client"

import { useMemo, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { Skeleton, Switch, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import {
  deriveProvider,
  providerMeta,
  type Connection,
  type RagSource,
} from "../../knowledge/_lib"
import { ProviderIcon } from "../../knowledge/_provider-icon"

const EMPTY_CONNECTIONS: Connection[] = []

export function KnowledgeSourcesTab({ channelId }: { channelId: string }) {
  const queryClient = useQueryClient()

  const sourcesQuery = $api.useQuery("get", "/v1/rag/sources", {
    params: { query: { page_size: 100 } },
  })
  const connectionsQuery = $api.useQuery("get", "/v1/connections", {
    params: { query: { limit: 100 } },
  })
  const channelSourcesQuery = $api.useQuery(
    "get",
    "/v1/channels/{id}/rag-sources",
    { params: { path: { id: channelId } } }
  )
  const setSources = $api.useMutation("put", "/v1/channels/{id}/rag-sources")

  const sources = useMemo(
    () => (sourcesQuery.data?.data ?? []) as RagSource[],
    [sourcesQuery.data?.data]
  )
  const connectionsById = useMemo(() => {
    const map = new Map<string, Connection>()
    for (const conn of connectionsQuery.data?.data ?? EMPTY_CONNECTIONS) {
      if (conn.id) map.set(conn.id, conn)
    }
    return map
  }, [connectionsQuery.data?.data])

  // Local optimistic set of enabled source IDs, seeded from the server and
  // re-seeded whenever the channel's sources refetch.
  const serverEnabled = useMemo(
    () =>
      new Set(
        (channelSourcesQuery.data?.data ?? [])
          .map((source) => source.id)
          .filter((id): id is string => Boolean(id))
      ),
    [channelSourcesQuery.data?.data]
  )
  const [enabled, setEnabled] = useState<Set<string> | null>(null)
  const [hydratedKey, setHydratedKey] = useState<string | null>(null)
  const serverKey = [...serverEnabled].sort().join(",")
  if (!channelSourcesQuery.isLoading && hydratedKey !== serverKey) {
    setHydratedKey(serverKey)
    setEnabled(new Set(serverEnabled))
  }
  const active = enabled ?? serverEnabled

  function toggle(sourceId: string, sourceName: string, on: boolean) {
    const previous = active
    const next = new Set(active)
    if (on) next.add(sourceId)
    else next.delete(sourceId)
    setEnabled(next)
    setSources.mutate(
      {
        params: { path: { id: channelId } },
        body: { source_ids: [...next] },
      },
      {
        onSuccess: () => {
          toast.success(
            on ? `Enabled ${sourceName}` : `Disabled ${sourceName}`
          )
          queryClient.invalidateQueries({
            queryKey: ["get", "/v1/channels/{id}/rag-sources"],
          })
        },
        onError: (error) => {
          // Roll back the optimistic flip.
          setEnabled(new Set(previous))
          toast.danger(
            extractErrorMessage(error, "Could not update knowledge sources")
          )
        },
      }
    )
  }

  const isLoading =
    sourcesQuery.isLoading ||
    connectionsQuery.isLoading ||
    channelSourcesQuery.isLoading

  if (isLoading) {
    return <SourcesSkeleton />
  }
  if (sourcesQuery.isError) {
    return (
      <div className="flex min-h-40 flex-col items-center justify-center rounded-xl bg-card px-6 text-center">
        <AppIcon
          icon="triangle-alert"
          className="h-7 w-7 text-muted-foreground"
        />
        <p className="mt-3 text-sm font-medium text-foreground">
          Could not load knowledge sources
        </p>
      </div>
    )
  }
  if (sources.length === 0) {
    return (
      <div className="flex min-h-40 flex-col items-center justify-center rounded-xl bg-card px-6 text-center">
        <AppIcon icon="folder-open" className="h-7 w-7 text-muted-foreground" />
        <p className="mt-3 text-sm font-medium text-foreground">
          No knowledge sources yet
        </p>
        <p className="mt-1 max-w-sm text-sm text-muted-foreground">
          Add sources in Knowledge settings, then enable them per channel here.
        </p>
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-3">
      <p className="text-sm text-muted-foreground">
        Agents in this channel can only search the sources enabled below.
      </p>
      <div className="flex flex-col gap-2">
        {sources.map((source) => {
          const id = source.id ?? ""
          const on = active.has(id)
          const provider = providerMeta(deriveProvider(source, connectionsById))
          return (
            <div
              key={id || source.name}
              className="flex min-w-0 items-center justify-between gap-4 rounded-lg border border-border bg-card p-3"
            >
              <div className="flex min-w-0 flex-1 items-center gap-3">
                <span className="bg-default flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-muted-foreground">
                  <ProviderIcon icon={provider.icon} className="h-4 w-4" />
                </span>
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium text-foreground">
                    {source.name}
                  </p>
                  <p className="truncate text-xs text-muted-foreground">
                    {provider.label}
                  </p>
                </div>
              </div>
              <Switch
                aria-label={`${on ? "Disable" : "Enable"} ${source.name}`}
                isSelected={on}
                isDisabled={!id}
                onChange={(value) => toggle(id, source.name ?? "source", value)}
                className="shrink-0"
              >
                <Switch.Control>
                  <Switch.Thumb />
                </Switch.Control>
              </Switch>
            </div>
          )
        })}
      </div>
    </div>
  )
}

function SourcesSkeleton() {
  return (
    <div className="flex flex-col gap-2">
      {Array.from({ length: 3 }).map((_, index) => (
        <div
          key={index}
          className="flex items-center justify-between gap-4 rounded-lg border border-border bg-card p-3"
        >
          <div className="flex min-w-0 flex-1 items-center gap-3">
            <Skeleton className="h-8 w-8" />
            <div className="flex min-w-0 flex-1 flex-col gap-2">
              <Skeleton className="h-4 w-32 rounded" />
              <Skeleton className="h-3 w-24 rounded" />
            </div>
          </div>
          <Skeleton className="h-6 w-10 rounded-full" />
        </div>
      ))}
    </div>
  )
}
