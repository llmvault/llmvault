"use client"

import { useMemo, useState } from "react"
import NextLink from "next/link"
import { Input, Skeleton } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { $api } from "@/lib/api/hooks"

type Channel = {
  id?: string
  name?: string
  description?: string
  member_count?: number
  is_default?: boolean
  visibility?: string
}

export default function ChannelsSettingsPage() {
  const [query, setQuery] = useState("")
  const channelsQuery = $api.useQuery("get", "/v1/channels", {
    params: { query: { limit: 200 } },
  })
  const channels = useMemo(
    () => (channelsQuery.data?.data ?? []) as Channel[],
    [channelsQuery.data?.data]
  )
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return channels
    return channels.filter(
      (channel) =>
        (channel.name ?? "").toLowerCase().includes(q) ||
        (channel.description ?? "").toLowerCase().includes(q)
    )
  }, [channels, query])

  return (
    <div className="flex flex-col gap-8">
      <div>
        <h1 className="text-2xl font-semibold text-foreground">Channels</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Every workspace channel. Open one to edit its agent and members, or to
          delete it.
        </p>
      </div>

      <div className="relative">
        <AppIcon
          icon="search"
          className="pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2 text-muted-foreground"
        />
        <Input
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          placeholder="Search channels"
          className="h-10 w-full rounded-md bg-card pl-9"
        />
      </div>

      {channelsQuery.isLoading ? (
        <LoadingState />
      ) : channelsQuery.isError ? (
        <ErrorState />
      ) : filtered.length === 0 ? (
        <EmptyState query={query} />
      ) : (
        <div className="flex flex-col gap-2">
          {filtered.map((channel) => (
            <ChannelCard key={channel.id} channel={channel} />
          ))}
        </div>
      )}
    </div>
  )
}

function ChannelCard({ channel }: { channel: Channel }) {
  return (
    <NextLink
      href={`/w/settings/channels/${channel.id}`}
      className="group flex items-center gap-3 rounded-xl border border-border bg-surface px-4 py-3 transition-colors hover:bg-default"
    >
      <span className="bg-default flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-muted-foreground">
        <AppIcon icon="hash" className="h-4 w-4" />
      </span>
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 items-center gap-2">
          <h2 className="truncate text-sm font-medium text-foreground">
            {channel.name}
          </h2>
          {channel.is_default ? (
            <span className="rounded-md bg-default px-1.5 py-0.5 text-xs text-muted-foreground">
              Default
            </span>
          ) : null}
          {channel.visibility === "private" ? (
            <span className="rounded-md bg-default px-1.5 py-0.5 text-xs text-muted-foreground">
              Private
            </span>
          ) : null}
        </div>
        <p className="truncate text-sm text-muted-foreground">
          {channel.description || "No description"}
        </p>
      </div>
      <span className="shrink-0 text-xs text-muted-foreground">
        {channel.member_count ?? 0}{" "}
        {(channel.member_count ?? 0) === 1 ? "member" : "members"}
      </span>
      <AppIcon
        icon="chevron-right"
        className="h-4 w-4 shrink-0 text-muted-foreground transition-colors group-hover:text-foreground"
      />
    </NextLink>
  )
}

function LoadingState() {
  return (
    <div className="flex flex-col gap-2">
      {Array.from({ length: 4 }).map((_, index) => (
        <div
          key={index}
          className="flex items-center gap-3 rounded-xl border border-border bg-surface px-4 py-3"
        >
          <Skeleton className="h-9 w-9 rounded-lg" />
          <div className="min-w-0 flex-1">
            <Skeleton className="h-4 w-40 rounded" />
            <Skeleton className="mt-2 h-4 w-full max-w-sm rounded" />
          </div>
        </div>
      ))}
    </div>
  )
}

function ErrorState() {
  return (
    <div className="flex min-h-56 flex-col items-center justify-center rounded-xl bg-card px-6 text-center">
      <AppIcon icon="triangle-alert" className="h-7 w-7 text-muted-foreground" />
      <p className="mt-3 text-sm font-medium text-foreground">
        Could not load channels
      </p>
      <p className="mt-1 max-w-sm text-sm text-muted-foreground">
        Refresh the page to try again.
      </p>
    </div>
  )
}

function EmptyState({ query }: { query: string }) {
  return (
    <div className="flex min-h-56 flex-col items-center justify-center rounded-xl bg-card px-6 text-center">
      <AppIcon icon="hash" className="h-7 w-7 text-muted-foreground" />
      <p className="mt-3 text-sm font-medium text-foreground">
        {query ? "No matching channels" : "No channels yet"}
      </p>
      <p className="mt-1 max-w-sm text-sm text-muted-foreground">
        {query
          ? "Try a different search."
          : "Channels you create appear here."}
      </p>
    </div>
  )
}
