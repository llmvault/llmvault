"use client"

import { useMemo, useState } from "react"
import { useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Spinner, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import {
  CHAT_QUERY_STALE_TIME_MS,
  invalidateSessionListQueries,
} from "@/app/w/(chat)/_lib/chat-cache"
import {
  channelDisplayName,
  channelRouteSlug,
  dedupeSessions,
  sessionActivityLabel,
  sessionDisplayName,
  type SidebarChannelResponse,
  type SidebarSessionResponse,
} from "@/app/w/(chat)/_lib/sidebar-data"

const ARCHIVED_SESSION_PAGE_LIMIT = 50
const ARCHIVED_SESSIONS_INFINITE_KEY = "archived-sessions-infinite-v1"
const ARCHIVED_CHANNEL_PAGE_LIMIT = 100

type ArchivedGroup = {
  channel: SidebarChannelResponse | null
  sessions: SidebarSessionResponse[]
}

export default function ArchivedSettingsPage() {
  const queryClient = useQueryClient()
  const router = useRouter()
  const [restoredIDs, setRestoredIDs] = useState<ReadonlySet<string>>(new Set())
  const [restoringID, setRestoringID] = useState<string | null>(null)
  const sessionsQuery = $api.useInfiniteQuery(
    "get",
    "/v1/sessions",
    {
      _hivyQueryKey: ARCHIVED_SESSIONS_INFINITE_KEY,
      params: {
        query: {
          status: "archived",
          limit: ARCHIVED_SESSION_PAGE_LIMIT,
          sort: "activity",
        },
      },
    },
    {
      initialPageParam: "0",
      pageParamName: "cursor",
      getNextPageParam: (lastPage) =>
        lastPage.has_more ? lastPage.next_cursor : undefined,
      retry: false,
      staleTime: CHAT_QUERY_STALE_TIME_MS,
    }
  )
  const channelsQuery = $api.useQuery(
    "get",
    "/v1/channels",
    { params: { query: { limit: ARCHIVED_CHANNEL_PAGE_LIMIT } } },
    { retry: false, staleTime: CHAT_QUERY_STALE_TIME_MS }
  )
  const restoreSession = $api.useMutation("patch", "/v1/sessions/{id}")

  const sessions = useMemo(
    () =>
      dedupeSessions(
        sessionsQuery.data?.pages.flatMap((page) => page.data ?? []) ?? []
      ).filter((session) => session.id && !restoredIDs.has(session.id)),
    [restoredIDs, sessionsQuery.data?.pages]
  )
  const channels = useMemo(
    () => channelsQuery.data?.data ?? [],
    [channelsQuery.data?.data]
  )
  const groups = useMemo(
    () => groupSessionsByChannel(sessions, channels),
    [channels, sessions]
  )

  function openSession(channel: SidebarChannelResponse, sessionID: string) {
    router.push(`/w/channels/${channelRouteSlug(channel)}/${sessionID}`)
  }

  function restore(sessionID: string) {
    if (!sessionID || restoringID) return
    setRestoringID(sessionID)
    restoreSession.mutate(
      {
        params: { path: { id: sessionID } },
        body: { status: "active" },
      },
      {
        onSuccess: () => {
          setRestoredIDs((current) => new Set(current).add(sessionID))
          invalidateSessionListQueries(queryClient)
          toast.success("Chat restored")
        },
        onError: (error) =>
          toast.danger(extractErrorMessage(error, "Could not restore chat")),
        onSettled: () => setRestoringID(null),
      }
    )
  }

  const loading = sessionsQuery.isLoading || channelsQuery.isLoading

  return (
    <div className="flex flex-col gap-8">
      <div>
        <h1 className="text-2xl font-semibold text-foreground">
          Archived chats
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Restore an archived chat to bring it back to your sidebar.
        </p>
      </div>

      {loading ? (
        <div className="flex min-h-40 items-center justify-center">
          <Spinner size="sm" />
        </div>
      ) : sessionsQuery.isError ? (
        <div className="flex flex-col items-center gap-3 rounded-2xl border border-border bg-surface px-6 py-12 text-center">
          <p className="text-sm text-muted-foreground">
            Could not load archived chats
          </p>
          <Button
            variant="tertiary"
            size="sm"
            onPress={() => void sessionsQuery.refetch()}
          >
            Retry
          </Button>
        </div>
      ) : groups.length === 0 ? (
        <div className="flex flex-col items-center gap-3 rounded-2xl border border-border bg-surface px-6 py-16 text-center">
          <div className="flex h-10 w-10 items-center justify-center rounded-xl border border-border bg-default">
            <AppIcon icon="archive" className="h-5 w-5 text-muted-foreground" />
          </div>
          <p className="text-sm text-muted-foreground">No archived chats</p>
        </div>
      ) : (
        <div className="flex flex-col gap-8">
          {groups.map((group) => (
            <ArchivedChannelGroup
              key={group.channel?.id ?? "direct"}
              group={group}
              restoringID={restoringID}
              onOpen={(sessionID) => {
                if (!group.channel) return
                openSession(group.channel, sessionID)
              }}
              onRestore={restore}
            />
          ))}
          {sessionsQuery.hasNextPage ? (
            <Button
              variant="tertiary"
              size="sm"
              className="self-start"
              isDisabled={sessionsQuery.isFetchingNextPage}
              onPress={() => void sessionsQuery.fetchNextPage()}
            >
              {sessionsQuery.isFetchingNextPage
                ? "Loading chats..."
                : "Show more chats"}
            </Button>
          ) : null}
        </div>
      )}
    </div>
  )
}

function ArchivedChannelGroup({
  group,
  restoringID,
  onOpen,
  onRestore,
}: {
  group: ArchivedGroup
  restoringID: string | null
  onOpen: (sessionID: string) => void
  onRestore: (sessionID: string) => void
}) {
  return (
    <section className="flex flex-col gap-3">
      <h2 className="flex items-center gap-2 text-sm font-semibold text-foreground">
        <AppIcon
          icon={group.channel ? "hash" : "message-circle"}
          className="h-4 w-4 shrink-0 text-muted-foreground"
        />
        {group.channel ? channelDisplayName(group.channel) : "Direct"}
      </h2>
      <div className="rounded-2xl border border-border bg-surface">
        {group.sessions.map((session, index) => {
          const id = session.id ?? ""
          return (
            <ArchivedSessionRow
              key={id}
              session={session}
              canOpen={Boolean(group.channel)}
              restoring={restoringID === id}
              last={index === group.sessions.length - 1}
              onOpen={() => onOpen(id)}
              onRestore={() => onRestore(id)}
            />
          )
        })}
      </div>
    </section>
  )
}

function ArchivedSessionRow({
  session,
  canOpen,
  restoring,
  last,
  onOpen,
  onRestore,
}: {
  session: SidebarSessionResponse
  canOpen: boolean
  restoring: boolean
  last?: boolean
  onOpen: () => void
  onRestore: () => void
}) {
  const meta = sessionActivityLabel(session)
  return (
    <div
      className={`flex items-center gap-4 px-4 py-3 ${last ? "" : "border-b border-border"}`}
    >
      <button
        type="button"
        disabled={!canOpen}
        onClick={onOpen}
        className="flex min-w-0 flex-1 items-center gap-3 text-left disabled:cursor-default"
      >
        <span className="min-w-0 flex-1 truncate text-sm font-medium">
          {sessionDisplayName(session)}
        </span>
        {meta ? (
          <span className="shrink-0 text-xs text-muted-foreground">{meta}</span>
        ) : null}
      </button>
      <Button
        variant="tertiary"
        size="sm"
        isDisabled={restoring}
        onPress={onRestore}
      >
        {restoring ? <Spinner color="current" size="sm" /> : null}
        Restore
      </Button>
    </div>
  )
}

function groupSessionsByChannel(
  sessions: SidebarSessionResponse[],
  channels: SidebarChannelResponse[]
): ArchivedGroup[] {
  const channelsByID = new Map(
    channels.flatMap((channel) =>
      channel.id ? ([[channel.id, channel]] as const) : []
    )
  )
  const byChannelID = new Map<string, SidebarSessionResponse[]>()
  const direct: SidebarSessionResponse[] = []
  for (const session of sessions) {
    const channel = session.channel_id
      ? channelsByID.get(session.channel_id)
      : undefined
    if (!channel?.id) {
      direct.push(session)
      continue
    }
    const list = byChannelID.get(channel.id) ?? []
    list.push(session)
    byChannelID.set(channel.id, list)
  }
  const groups: ArchivedGroup[] = []
  for (const channel of channels) {
    const list = channel.id ? byChannelID.get(channel.id) : undefined
    if (list?.length) groups.push({ channel, sessions: list })
  }
  if (direct.length) groups.push({ channel: null, sessions: direct })
  return groups
}
