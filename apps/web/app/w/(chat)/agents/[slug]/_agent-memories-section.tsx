"use client"

import { useMemo, useState, type FormEvent } from "react"
import { useQueryClient } from "@tanstack/react-query"
import {
  Button,
  Popover,
  Skeleton,
  Spinner,
  TextArea,
  toast,
} from "@heroui/react"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { queryKeys } from "@/lib/api/query-keys"
import type { components } from "@/lib/api/schema"

type MemoryResponse = components["schemas"]["memoryResponse"]

type AgentMemoryCardData = {
  id: string
  content: string
  tags: string[]
  createdAt: string
}

export function toAgentMemory(memory: MemoryResponse): AgentMemoryCardData {
  return {
    id: memory.id ?? "",
    content: memory.content?.trim() ?? "",
    tags: Array.from(
      new Set(
        (memory.tags ?? [])
          .map((tag) => tag.trim())
          .filter((tag) => tag.length > 0)
      )
    ),
    createdAt: memory.created_at ?? "",
  }
}

export function AgentMemoriesSection({ agentId }: { agentId: string }) {
  const queryClient = useQueryClient()
  const memoriesQuery = $api.useQuery("get", "/v1/memories", {
    params: { query: { agent_id: agentId, limit: 100 } },
  })
  const updateMemory = $api.useMutation("patch", "/v1/memories/{id}")
  const forgetMemory = $api.useMutation("delete", "/v1/memories/{id}")
  const memories = useMemo(
    () =>
      (memoriesQuery.data?.data ?? [])
        .map(toAgentMemory)
        .filter((memory) => memory.id && memory.content),
    [memoriesQuery.data?.data]
  )
  const invalidateMemories = () =>
    queryClient.invalidateQueries({ queryKey: queryKeys.memories() })

  async function edit(memory: AgentMemoryCardData, content: string) {
    try {
      await updateMemory.mutateAsync({
        params: { path: { id: memory.id } },
        body: { content },
      })
      await invalidateMemories()
      toast.success("Memory updated")
    } catch (error) {
      toast.danger(extractErrorMessage(error, "Could not update memory"))
      throw error
    }
  }

  async function forget(memory: AgentMemoryCardData) {
    try {
      await forgetMemory.mutateAsync({
        params: { path: { id: memory.id } },
      })
      await invalidateMemories()
      toast.success("Memory forgotten")
    } catch (error) {
      toast.danger(extractErrorMessage(error, "Could not forget memory"))
      throw error
    }
  }

  return (
    <AgentMemoriesView
      memories={memories}
      isLoading={memoriesQuery.isLoading}
      isError={memoriesQuery.isError}
      actionsDisabled={updateMemory.isPending || forgetMemory.isPending}
      onRetry={() => void memoriesQuery.refetch()}
      onEdit={edit}
      onForget={forget}
    />
  )
}

export function AgentMemoriesView({
  memories,
  isLoading = false,
  isError = false,
  actionsDisabled = false,
  onRetry,
  onEdit,
  onForget,
}: {
  memories: AgentMemoryCardData[]
  isLoading?: boolean
  isError?: boolean
  actionsDisabled?: boolean
  onRetry?: () => void
  onEdit: (memory: AgentMemoryCardData, content: string) => Promise<void>
  onForget: (memory: AgentMemoryCardData) => Promise<void>
}) {
  const [forgetTarget, setForgetTarget] = useState<AgentMemoryCardData | null>(
    null
  )
  const [forgetPending, setForgetPending] = useState(false)

  async function confirmForget() {
    if (!forgetTarget || forgetPending) return
    setForgetPending(true)
    try {
      await onForget(forgetTarget)
      setForgetTarget(null)
    } catch {
      // The mutation handler shows the error. Keep the dialog open to retry.
    } finally {
      setForgetPending(false)
    }
  }

  return (
    <>
      <section
        className="flex flex-col gap-4"
        aria-labelledby="agent-memories-heading"
      >
        <div className="flex items-start justify-between gap-4">
          <div>
            <h2
              id="agent-memories-heading"
              className="text-sm font-semibold text-foreground"
            >
              Learned memories
            </h2>
            <p className="text-muted-foreground mt-1 max-w-2xl text-sm leading-5">
              Durable facts this agent has captured automatically from its
              sessions.
            </p>
          </div>
          {memories.length > 0 && !isLoading && !isError ? (
            <span className="text-muted-foreground shrink-0 text-xs">
              {memories.length} {memories.length === 1 ? "memory" : "memories"}
            </span>
          ) : null}
        </div>

        {isLoading ? (
          <MemorySkeletons />
        ) : isError ? (
          <div className="bg-card flex min-h-48 flex-col items-center justify-center rounded-xl px-6 text-center">
            <AppIcon
              icon="triangle-alert"
              className="text-muted-foreground h-7 w-7"
            />
            <p className="mt-3 text-sm font-medium text-foreground">
              Could not load memories
            </p>
            <p className="text-muted-foreground mt-1 max-w-sm text-sm">
              Try again to reload what this agent remembers.
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
        ) : memories.length === 0 ? (
          <div className="bg-card flex min-h-56 flex-col items-center justify-center rounded-xl px-6 text-center">
            <AppIcon icon="brain" className="text-muted-foreground h-7 w-7" />
            <p className="mt-3 text-sm font-medium text-foreground">
              No memories yet
            </p>
            <p className="text-muted-foreground mt-1 max-w-sm text-sm leading-5">
              Memories will appear here as this agent learns durable facts from
              its sessions.
            </p>
          </div>
        ) : (
          <div className="flex flex-col gap-2">
            {memories.map((memory) => (
              <MemoryCard
                key={memory.id}
                memory={memory}
                actionsDisabled={actionsDisabled}
                onEdit={onEdit}
                onForget={() => setForgetTarget(memory)}
              />
            ))}
          </div>
        )}
      </section>

      <ConfirmDialog
        open={forgetTarget !== null}
        pending={forgetPending}
        heading="Forget memory"
        description="This removes the memory from this agent's recall. This action cannot be undone."
        confirmLabel="Forget memory"
        onOpenChange={(open) => {
          if (!open && !forgetPending) setForgetTarget(null)
        }}
        onConfirm={() => void confirmForget()}
      />
    </>
  )
}

function MemoryCard({
  memory,
  actionsDisabled,
  onEdit,
  onForget,
}: {
  memory: AgentMemoryCardData
  actionsDisabled: boolean
  onEdit: (memory: AgentMemoryCardData, content: string) => Promise<void>
  onForget: () => void
}) {
  const [editing, setEditing] = useState(false)
  const [content, setContent] = useState(memory.content)
  const [saving, setSaving] = useState(false)
  const relativeCreatedAt = relativeTime(memory.createdAt)

  function startEditing() {
    setContent(memory.content)
    setEditing(true)
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const next = content.trim()
    if (!next || next === memory.content || saving) return
    setSaving(true)
    try {
      await onEdit(memory, next)
      setEditing(false)
    } catch {
      // The mutation handler shows the error. Keep the editor open to retry.
    } finally {
      setSaving(false)
    }
  }

  return (
    <article className="flex items-start gap-3 rounded-xl border border-border bg-surface px-4 py-3">
      <div className="min-w-0 flex-1">
        {editing ? (
          <form onSubmit={submit} className="flex flex-col gap-3">
            <TextArea
              autoFocus
              value={content}
              disabled={saving}
              aria-label="Memory content"
              rows={3}
              fullWidth
              onChange={(event) => setContent(event.target.value)}
            />
            <div className="flex items-center justify-end gap-2">
              <Button
                type="button"
                variant="tertiary"
                size="sm"
                isDisabled={saving}
                onPress={() => setEditing(false)}
              >
                Cancel
              </Button>
              <Button
                type="submit"
                variant="primary"
                size="sm"
                isDisabled={
                  !content.trim() || content.trim() === memory.content || saving
                }
              >
                {saving ? <Spinner color="current" size="sm" /> : null}
                Save
              </Button>
            </div>
          </form>
        ) : (
          <p className="text-sm leading-5 text-foreground">{memory.content}</p>
        )}
        {!editing && (memory.tags.length > 0 || relativeCreatedAt) ? (
          <div className="mt-2 flex flex-wrap items-center gap-1.5">
            {memory.tags.map((tag, index) => (
              <span
                key={`${memory.id}-${tag}-${index}`}
                className="text-muted-foreground rounded-md bg-default px-1.5 py-0.5 text-xs"
              >
                {tag}
              </span>
            ))}
            {relativeCreatedAt ? (
              <time
                dateTime={memory.createdAt}
                className="text-muted-foreground text-xs"
              >
                {memory.tags.length > 0 ? "· " : ""}
                {relativeCreatedAt}
              </time>
            ) : null}
          </div>
        ) : null}
      </div>
      {!editing ? (
        <MemoryActionsMenu
          disabled={actionsDisabled}
          onEdit={startEditing}
          onForget={onForget}
        />
      ) : null}
    </article>
  )
}

function MemoryActionsMenu({
  disabled,
  onEdit,
  onForget,
}: {
  disabled: boolean
  onEdit: () => void
  onForget: () => void
}) {
  const [open, setOpen] = useState(false)
  return (
    <Popover
      isOpen={open}
      onOpenChange={(next) => {
        if (!disabled) setOpen(next)
      }}
    >
      <Popover.Trigger
        aria-label="Memory options"
        aria-disabled={disabled || undefined}
        data-open={open ? "true" : undefined}
        className={`text-muted-foreground -mr-1 flex shrink-0 items-center rounded-md p-1 transition-colors data-[open=true]:bg-default ${
          disabled ? "cursor-not-allowed opacity-50" : "hover:bg-default"
        }`}
      >
        <AppIcon icon="ellipsis" className="h-4 w-4" />
      </Popover.Trigger>
      {open ? (
        <Popover.Content
          placement="bottom end"
          offset={6}
          className="w-44 rounded-2xl border border-border p-1.5"
        >
          <Popover.Dialog className="p-0">
            <MemoryActionsMenuContent
              disabled={disabled}
              onEdit={() => {
                setOpen(false)
                onEdit()
              }}
              onForget={() => {
                setOpen(false)
                onForget()
              }}
            />
          </Popover.Dialog>
        </Popover.Content>
      ) : null}
    </Popover>
  )
}

export function MemoryActionsMenuContent({
  disabled = false,
  onEdit,
  onForget,
}: {
  disabled?: boolean
  onEdit: () => void
  onForget: () => void
}) {
  return (
    <div className="flex w-full flex-col gap-0.5">
      <button
        type="button"
        disabled={disabled}
        onClick={onEdit}
        className="flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors hover:bg-default disabled:opacity-50"
      >
        <AppIcon icon="pencil" className="h-4 w-4 shrink-0" />
        Edit
      </button>
      <button
        type="button"
        disabled={disabled}
        onClick={onForget}
        className="flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-sm text-danger transition-colors hover:bg-danger/10 disabled:opacity-50"
      >
        <AppIcon icon="trash-2" className="h-4 w-4 shrink-0" />
        Forget
      </button>
    </div>
  )
}

function MemorySkeletons() {
  return (
    <div className="flex flex-col gap-2" aria-label="Loading memories">
      {Array.from({ length: 3 }).map((_, row) => (
        <div
          key={row}
          className="flex flex-col gap-2 rounded-xl border border-border bg-surface px-4 py-3"
        >
          <Skeleton className="h-3.5 w-full max-w-xl rounded" />
          <div className="flex items-center gap-1.5">
            <Skeleton className="h-4 w-14 rounded-md" />
            <Skeleton className="h-4 w-16 rounded-md" />
          </div>
        </div>
      ))}
    </div>
  )
}

export function relativeTime(iso: string, now = Date.now()): string {
  if (!iso) return ""
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return ""
  const seconds = Math.max(0, Math.floor((now - then) / 1000))
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
