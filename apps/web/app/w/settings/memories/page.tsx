"use client"

import { useEffect, useMemo, useState, type ComponentProps } from "react"
import { useQueryClient } from "@tanstack/react-query"
import {
  Input,
  ListBox,
  Popover,
  Select,
  Skeleton,
  Spinner,
  toast,
} from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { api } from "@/lib/api/client"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"

type Memory = {
  id: string
  content: string
  tags: string[]
  createdAt: string
  channelId: string | null
}

type GroupState = {
  key: string // channelId, or "global"
  channelId: string | null
  channelName: string
  memories: Memory[]
  hasMore: boolean
  cursor: string | null
  loadingMore: boolean
}

const PER_CHANNEL = 10

export default function MemoriesSettingsPage() {
  const queryClient = useQueryClient()
  const groupedQuery = $api.useQuery("get", "/v1/memories/grouped", {
    params: { query: { per_channel: PER_CHANNEL } },
  })
  const [groups, setGroups] = useState<GroupState[]>([])
  const [query, setQuery] = useState("")
  const [channel, setChannel] = useState("all")

  useEffect(() => {
    if (!groupedQuery.data?.groups) return
    setGroups(groupedQuery.data.groups.map(toGroupState))
  }, [groupedQuery.data])

  const invalidate = () =>
    queryClient.invalidateQueries({
      queryKey: ["get", "/v1/memories/grouped"],
    })

  const forgetMutation = $api.useMutation("delete", "/v1/memories/{id}")
  const updateMutation = $api.useMutation("patch", "/v1/memories/{id}")

  function forget(memoryId: string) {
    forgetMutation.mutate(
      { params: { path: { id: memoryId } } },
      {
        onSuccess: () => {
          toast.success("Memory forgotten")
          invalidate()
        },
        onError: (error) =>
          toast.danger(extractErrorMessage(error, "Could not forget memory")),
      }
    )
  }

  function makeGlobal(memoryId: string) {
    updateMutation.mutate(
      { params: { path: { id: memoryId } }, body: { channel_id: "org" } },
      {
        onSuccess: () => {
          toast.success("Memory is now global")
          invalidate()
        },
        onError: (error) =>
          toast.danger(extractErrorMessage(error, "Could not update memory")),
      }
    )
  }

  async function loadMore(key: string) {
    const group = groups.find((g) => g.key === key)
    if (!group || group.loadingMore) return
    setGroups((current) =>
      current.map((g) => (g.key === key ? { ...g, loadingMore: true } : g))
    )
    try {
      const { data, error } = await api.GET(
        "/v1/memories/channels/{channelId}",
        {
          params: {
            path: { channelId: group.channelId ?? "global" },
            query: { cursor: group.cursor ?? undefined, limit: PER_CHANNEL },
          },
        }
      )
      if (error) throw error
      setGroups((current) =>
        current.map((g) =>
          g.key === key
            ? {
                ...g,
                memories: [...g.memories, ...(data?.data ?? []).map(toMemory)],
                cursor: data?.next_cursor ?? null,
                hasMore: Boolean(data?.has_more),
                loadingMore: false,
              }
            : g
        )
      )
    } catch (error) {
      setGroups((current) =>
        current.map((g) => (g.key === key ? { ...g, loadingMore: false } : g))
      )
      toast.danger(extractErrorMessage(error, "Could not load more memories"))
    }
  }

  const channelOptions = useMemo(
    () => [
      { id: "all", label: "All channels" },
      ...groups.map((group) => ({ id: group.key, label: group.channelName })),
    ],
    [groups]
  )
  const filtered = useMemo(
    () => filterGroups(groups, query, channel),
    [groups, query, channel]
  )

  return (
    <div className="flex flex-col gap-8">
      <div>
        <h1 className="text-2xl font-semibold text-foreground">Memories</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Durable facts your agents remember, grouped by the channel they belong
          to. Global memories are shared with every channel set to expose them.
        </p>
      </div>

      <div className="flex w-full flex-col gap-3 sm:flex-row sm:items-center">
        <div className="relative min-w-0 flex-1">
          <AppIcon
            icon="search"
            className="pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Search loaded memories"
            className="h-10 w-full rounded-md bg-card pl-9"
          />
        </div>
        <ChannelSelect
          options={channelOptions}
          value={channel}
          onChange={setChannel}
        />
      </div>

      {groupedQuery.isLoading ? (
        <LoadingState />
      ) : groupedQuery.isError ? (
        <ErrorState />
      ) : filtered.length === 0 ? (
        <EmptyState query={query} hasAny={groups.length > 0} />
      ) : (
        <div className="flex flex-col gap-8">
          {filtered.map((group) => (
            <ChannelSection
              key={group.key}
              group={group}
              onForget={forget}
              onMakeGlobal={makeGlobal}
              onLoadMore={() => loadMore(group.key)}
            />
          ))}
        </div>
      )}
    </div>
  )
}

function ChannelSection({
  group,
  onForget,
  onMakeGlobal,
  onLoadMore,
}: {
  group: GroupState
  onForget: (memoryId: string) => void
  onMakeGlobal: (memoryId: string) => void
  onLoadMore: () => void
}) {
  const orgWide = group.channelId === null
  return (
    <section className="flex flex-col gap-3">
      <div className="flex items-center gap-2">
        <AppIcon
          icon={orgWide ? "globe" : "hash"}
          className="h-4 w-4 text-muted-foreground"
        />
        <h2 className="text-sm font-medium text-foreground">
          {group.channelName}
        </h2>
        <span className="text-xs text-muted-foreground">{group.memories.length}</span>
      </div>
      <div className="flex flex-col gap-2">
        {group.memories.map((memory) => (
          <MemoryCard
            key={memory.id}
            memory={memory}
            orgWide={orgWide}
            onForget={() => onForget(memory.id)}
            onMakeGlobal={() => onMakeGlobal(memory.id)}
          />
        ))}
      </div>
      {group.hasMore ? (
        <button
          type="button"
          onClick={onLoadMore}
          disabled={group.loadingMore}
          className="hover:bg-default flex items-center justify-center gap-2 self-start rounded-lg px-3 py-1.5 text-sm text-muted-foreground transition-colors disabled:opacity-60"
        >
          {group.loadingMore ? (
            <Spinner color="current" size="sm" />
          ) : (
            <AppIcon icon="chevron-down" className="h-4 w-4" />
          )}
          Load more
        </button>
      ) : null}
    </section>
  )
}

function MemoryCard({
  memory,
  orgWide,
  onForget,
  onMakeGlobal,
}: {
  memory: Memory
  orgWide: boolean
  onForget: () => void
  onMakeGlobal: () => void
}) {
  return (
    <div className="group flex items-start gap-3 rounded-xl border border-border bg-surface px-4 py-3">
      <div className="flex min-w-0 flex-1 flex-col gap-2">
        <p className="text-xs leading-5 text-foreground">{memory.content}</p>
        <div className="flex flex-wrap items-center gap-1.5">
          {memory.tags.map((tag) => (
            <span
              key={tag}
              className="rounded-md bg-default px-1.5 py-0.5 text-xs text-muted-foreground"
            >
              {tag}
            </span>
          ))}
          <span className="text-xs text-muted-foreground">
            {memory.tags.length > 0 ? "· " : ""}
            {relativeTime(memory.createdAt)}
          </span>
        </div>
      </div>
      <MemoryActionsMenu
        orgWide={orgWide}
        onForget={onForget}
        onMakeGlobal={onMakeGlobal}
      />
    </div>
  )
}

function MemoryActionsMenu({
  orgWide,
  onForget,
  onMakeGlobal,
  placement = "bottom end",
}: {
  orgWide: boolean
  onForget: () => void
  onMakeGlobal: () => void
  placement?: ComponentProps<typeof Popover.Content>["placement"]
}) {
  const [open, setOpen] = useState(false)
  function select(action: "global" | "forget") {
    if (action === "global") onMakeGlobal()
    else onForget()
    setOpen(false)
  }
  return (
    <Popover isOpen={open} onOpenChange={setOpen}>
      <Popover.Trigger
        aria-label="Memory options"
        data-open={open ? "true" : undefined}
        className="hover:bg-default data-[open=true]:bg-default -mr-1 flex shrink-0 items-center rounded-md p-1 text-muted-foreground transition-colors"
      >
        <AppIcon icon="ellipsis" className="h-4 w-4" />
      </Popover.Trigger>
      {open ? (
        <Popover.Content
          placement={placement}
          offset={6}
          className="w-52 rounded-2xl border border-border p-1.5"
        >
          <Popover.Dialog className="flex w-full flex-col gap-0.5 p-0">
            {!orgWide ? (
              <button
                type="button"
                onClick={() => select("global")}
                className="hover:bg-default flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors"
              >
                <AppIcon icon="globe" className="h-4 w-4 shrink-0" />
                Make global
              </button>
            ) : null}
            <button
              type="button"
              onClick={() => select("forget")}
              className="flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-sm text-danger transition-colors hover:bg-danger/10"
            >
              <AppIcon icon="trash-2" className="h-4 w-4 shrink-0" />
              Forget memory
            </button>
          </Popover.Dialog>
        </Popover.Content>
      ) : null}
    </Popover>
  )
}

function ChannelSelect({
  options,
  value,
  onChange,
}: {
  options: { id: string; label: string }[]
  value: string
  onChange: (value: string) => void
}) {
  return (
    <Select
      aria-label="Filter by channel"
      value={value}
      onChange={(key) => onChange(String(key))}
      className="w-full sm:w-52"
    >
      <Select.Trigger className="h-10 w-full justify-between px-3 text-sm transition-colors">
        <Select.Value />
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover className="w-52 p-1.5">
        <ListBox>
          {options.map((option) => (
            <ListBox.Item key={option.id} id={option.id} textValue={option.label}>
              {option.label}
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}

function LoadingState() {
  return (
    <div className="flex flex-col gap-8">
      {Array.from({ length: 2 }).map((_, section) => (
        <section key={section} className="flex flex-col gap-3">
          <div className="flex items-center gap-2">
            <Skeleton className="h-4 w-4 rounded" />
            <Skeleton className="h-4 w-32 rounded" />
          </div>
          <div className="flex flex-col gap-2">
            {Array.from({ length: section === 0 ? 2 : 3 }).map((_, row) => (
              <div
                key={row}
                className="flex items-start gap-3 rounded-xl border border-border bg-surface px-4 py-3"
              >
                <div className="flex min-w-0 flex-1 flex-col gap-2">
                  <Skeleton className="h-3.5 w-full max-w-md rounded" />
                  <div className="flex items-center gap-1.5">
                    <Skeleton className="h-4 w-14 rounded-md" />
                    <Skeleton className="h-4 w-16 rounded-md" />
                  </div>
                </div>
                <Skeleton className="h-6 w-6 rounded-md" />
              </div>
            ))}
          </div>
        </section>
      ))}
    </div>
  )
}

function ErrorState() {
  return (
    <div className="flex min-h-56 flex-col items-center justify-center rounded-xl bg-card px-6 text-center">
      <AppIcon icon="triangle-alert" className="h-7 w-7 text-muted-foreground" />
      <p className="mt-3 text-sm font-medium text-foreground">
        Could not load memories
      </p>
      <p className="mt-1 max-w-sm text-sm text-muted-foreground">
        Refresh the page to try again.
      </p>
    </div>
  )
}

function EmptyState({ query, hasAny }: { query: string; hasAny: boolean }) {
  return (
    <div className="flex min-h-56 flex-col items-center justify-center rounded-xl bg-card px-6 text-center">
      <AppIcon icon="brain" className="h-7 w-7 text-muted-foreground" />
      <p className="mt-3 text-sm font-medium text-foreground">
        {query ? "No matching memories" : "No memories yet"}
      </p>
      <p className="mt-1 max-w-sm text-sm text-muted-foreground">
        {query
          ? "Try a different search."
          : hasAny
            ? "All memories were removed."
            : "Your agents will remember durable facts here as they work in each channel."}
      </p>
    </div>
  )
}

type ApiGroup = {
  channel_id?: string
  channel_name?: string
  memories?: ApiMemory[]
  has_more?: boolean
  total?: number
  next_cursor?: string
}
type ApiMemory = {
  id?: string
  content?: string
  tags?: string[]
  created_at?: string
  channel_id?: string
}

function toGroupState(group: ApiGroup): GroupState {
  const channelId = group.channel_id ?? null
  return {
    key: channelId ?? "global",
    channelId,
    channelName: group.channel_name ?? "Global",
    memories: (group.memories ?? []).map(toMemory),
    hasMore: Boolean(group.has_more),
    cursor: group.next_cursor ?? null,
    loadingMore: false,
  }
}

function toMemory(memory: ApiMemory): Memory {
  return {
    id: memory.id ?? "",
    content: memory.content ?? "",
    tags: memory.tags ?? [],
    createdAt: memory.created_at ?? "",
    channelId: memory.channel_id ?? null,
  }
}

function filterGroups(
  groups: GroupState[],
  query: string,
  channel: string
): GroupState[] {
  const q = query.trim().toLowerCase()
  return groups
    .filter((group) => channel === "all" || group.key === channel)
    .map((group) => ({
      ...group,
      memories: !q
        ? group.memories
        : group.memories.filter(
            (memory) =>
              memory.content.toLowerCase().includes(q) ||
              memory.tags.some((tag) => tag.toLowerCase().includes(q))
          ),
    }))
    .filter(
      (group) => group.memories.length > 0 || (channel === group.key && !q)
    )
}

function relativeTime(iso: string): string {
  if (!iso) return ""
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return ""
  const seconds = Math.max(0, Math.floor((Date.now() - then) / 1000))
  if (seconds < 60) return "just now"
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  if (days < 30) return `${days}d ago`
  const months = Math.floor(days / 30)
  if (months < 12) return `${months}mo ago`
  return `${Math.floor(months / 12)}y ago`
}
