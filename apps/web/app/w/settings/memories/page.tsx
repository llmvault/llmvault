"use client"

import { useMemo, useState, type FormEvent } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Button, Input, Spinner, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import {
  confirmObservation,
  correctObservation,
  createDirective,
  deleteDirective,
  deleteObservation,
  listDirectives,
  listObservations,
  memoryQueryKeys,
  pinObservation,
  updateDirective,
  type Directive,
  type Observation,
} from "@/lib/api/memory"
import {
  ChannelSelect,
  DirectiveRow,
  EmptyCard,
  ErrorState,
  LoadingState,
  ObservationRow,
  RowSkeletons,
} from "./_memory-rows"
import {
  DeleteMemoryDialog,
  DeleteRuleDialog,
  EditContentModal,
} from "./_memory-dialogs"

type Channel = { id?: string; name?: string }

const PAGE_SIZE = 30

type ExtraPages = {
  channelId: string
  observations: Observation[]
  hasMore: boolean
}

export default function MemoriesSettingsPage() {
  const queryClient = useQueryClient()

  const channelsQuery = $api.useQuery("get", "/v1/channels", {
    params: { query: { limit: 200 } },
  })
  const channels = useMemo(
    () => (channelsQuery.data?.data ?? []) as Channel[],
    [channelsQuery.data?.data]
  )
  const [selectedChannelId, setSelectedChannelId] = useState("")
  const channelId = selectedChannelId || channels[0]?.id || ""

  const directivesQuery = useQuery({
    queryKey: memoryQueryKeys.directives(channelId),
    queryFn: () => listDirectives(channelId),
    enabled: Boolean(channelId),
  })
  const observationsQuery = useQuery({
    queryKey: memoryQueryKeys.observations(channelId),
    queryFn: () => listObservations(channelId, { limit: PAGE_SIZE }),
    enabled: Boolean(channelId),
  })

  // Pages beyond the first, accumulated locally; reset whenever the first
  // page refetches (mutations invalidate) or the channel changes.
  const [extraPages, setExtraPages] = useState<ExtraPages | null>(null)
  const [loadingMore, setLoadingMore] = useState(false)
  const extra = extraPages?.channelId === channelId ? extraPages : null

  const observations = useMemo(() => {
    const first = observationsQuery.data?.observations ?? []
    if (!extra) return first
    const seen = new Set(first.map((observation) => observation.id))
    return [
      ...first,
      ...extra.observations.filter((observation) => !seen.has(observation.id)),
    ]
  }, [observationsQuery.data, extra])
  const hasMore = extra?.hasMore ?? Boolean(observationsQuery.data?.hasMore)

  const directives = useMemo(
    () => (directivesQuery.data ?? []).filter((directive) => directive.active),
    [directivesQuery.data]
  )

  function invalidateObservations() {
    setExtraPages(null)
    queryClient.invalidateQueries({
      queryKey: memoryQueryKeys.observations(channelId),
    })
  }
  function invalidateDirectives() {
    queryClient.invalidateQueries({
      queryKey: memoryQueryKeys.directives(channelId),
    })
  }

  async function loadMore() {
    if (!channelId || !hasMore || loadingMore) return
    setLoadingMore(true)
    try {
      // Offset pagination: skip everything already fetched (raw page counts,
      // before display-side dedupe).
      const offset =
        (observationsQuery.data?.observations.length ?? 0) +
        (extra?.observations.length ?? 0)
      const page = await listObservations(channelId, {
        offset,
        limit: PAGE_SIZE,
      })
      setExtraPages({
        channelId,
        observations: [...(extra?.observations ?? []), ...page.observations],
        hasMore: page.hasMore,
      })
    } catch (error) {
      toast.danger(extractErrorMessage(error, "Could not load more memories"))
    } finally {
      setLoadingMore(false)
    }
  }

  // -------------------------------------------------------------------------
  // Observation actions
  // -------------------------------------------------------------------------

  const confirmMutation = useMutation({
    mutationFn: (id: string) => confirmObservation(id),
    onSuccess: () => {
      toast.success("Memory confirmed")
      invalidateObservations()
    },
    onError: (error) =>
      toast.danger(extractErrorMessage(error, "Could not confirm memory")),
  })

  const [editTarget, setEditTarget] = useState<Observation | null>(null)
  const correctMutation = useMutation({
    mutationFn: ({ id, content }: { id: string; content: string }) =>
      correctObservation(id, content),
    onSuccess: () => {
      toast.success("Memory corrected")
      setEditTarget(null)
      invalidateObservations()
    },
    onError: (error) =>
      toast.danger(extractErrorMessage(error, "Could not update memory")),
  })

  const [deleteTarget, setDeleteTarget] = useState<Observation | null>(null)
  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteObservation(id),
    onSuccess: () => {
      toast.success("Memory deleted")
      setDeleteTarget(null)
      invalidateObservations()
    },
    onError: (error) =>
      toast.danger(extractErrorMessage(error, "Could not delete memory")),
  })

  const pinMutation = useMutation({
    mutationFn: (id: string) => pinObservation(id),
    onSuccess: () => {
      toast.success("Pinned as rule")
      invalidateObservations()
      invalidateDirectives()
    },
    onError: (error) =>
      toast.danger(extractErrorMessage(error, "Could not pin memory")),
  })

  // -------------------------------------------------------------------------
  // Directive actions
  // -------------------------------------------------------------------------

  const [newRule, setNewRule] = useState("")
  const addRuleMutation = useMutation({
    mutationFn: (content: string) => createDirective(channelId, content),
    onSuccess: () => {
      toast.success("Rule added")
      setNewRule("")
      invalidateDirectives()
    },
    onError: (error) =>
      toast.danger(extractErrorMessage(error, "Could not add rule")),
  })

  // Rules are immutable text: no edit action. To change a rule, delete it
  // and add a new one.
  const [deleteRuleTarget, setDeleteRuleTarget] = useState<Directive | null>(
    null
  )
  const deleteRuleMutation = useMutation({
    mutationFn: (id: string) => deleteDirective(id),
    onSuccess: () => {
      toast.success("Rule deleted")
      setDeleteRuleTarget(null)
      invalidateDirectives()
    },
    onError: (error) =>
      toast.danger(extractErrorMessage(error, "Could not delete rule")),
  })

  const deactivateRuleMutation = useMutation({
    mutationFn: (id: string) => updateDirective(id, { active: false }),
    onSuccess: () => {
      toast.success("Rule deactivated")
      invalidateDirectives()
    },
    onError: (error) =>
      toast.danger(extractErrorMessage(error, "Could not deactivate rule")),
  })

  function submitRule(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const content = newRule.trim()
    if (!content || !channelId || addRuleMutation.isPending) return
    addRuleMutation.mutate(content)
  }

  const channelOptions = useMemo(
    () =>
      channels.map((channel) => ({
        id: channel.id ?? "",
        label: channel.name ?? "Channel",
      })),
    [channels]
  )

  return (
    <div className="flex flex-col gap-8">
      <div>
        <h1 className="text-2xl font-semibold text-foreground">Memories</h1>
        <p className="text-muted-foreground mt-1 text-sm">
          What this channel&apos;s agent remembers: rules you control, and
          memories it builds as it works. Confirm, correct, or delete anything.
        </p>
      </div>

      {channelsQuery.isLoading ? (
        <LoadingState />
      ) : channelsQuery.isError ? (
        <ErrorState label="Could not load channels" />
      ) : channels.length === 0 ? (
        <EmptyCard
          icon="hash"
          title="No channels yet"
          body="Create a channel and its memories will appear here."
        />
      ) : (
        <>
          <ChannelSelect
            options={channelOptions}
            value={channelId}
            onChange={setSelectedChannelId}
          />

          <section className="flex flex-col gap-3">
            <div>
              <h2 className="flex items-center gap-2 text-sm font-medium text-foreground">
                <AppIcon
                  icon="list-checks"
                  className="text-muted-foreground h-4 w-4"
                />
                Rules
                {directives.length > 0 ? (
                  <span className="text-muted-foreground text-xs font-normal">
                    {directives.length}
                  </span>
                ) : null}
              </h2>
              <p className="text-muted-foreground mt-1 text-sm">
                Rules are injected into every session in this channel.
              </p>
            </div>

            {directivesQuery.isLoading ? (
              <RowSkeletons count={2} />
            ) : directivesQuery.isError ? (
              <ErrorState label="Could not load rules" />
            ) : directives.length === 0 ? (
              <div className="text-muted-foreground rounded-xl border border-dashed border-border px-4 py-5 text-center text-sm">
                No rules yet. Add one below, or pin a memory as a rule.
              </div>
            ) : (
              <div className="flex flex-col gap-2">
                {directives.map((directive) => (
                  <DirectiveRow
                    key={directive.id}
                    directive={directive}
                    onDeactivate={() =>
                      deactivateRuleMutation.mutate(directive.id)
                    }
                    onDelete={() => setDeleteRuleTarget(directive)}
                  />
                ))}
              </div>
            )}

            <form onSubmit={submitRule} className="flex items-center gap-2">
              <Input
                value={newRule}
                onChange={(event) => setNewRule(event.target.value)}
                placeholder="Add a rule, e.g. Always reply in English"
                aria-label="New rule"
                className="h-10 min-w-0 flex-1"
              />
              <Button
                type="submit"
                variant="secondary"
                size="sm"
                className="shrink-0"
                isDisabled={
                  !newRule.trim() || !channelId || addRuleMutation.isPending
                }
              >
                {addRuleMutation.isPending ? (
                  <Spinner color="current" size="sm" />
                ) : (
                  <AppIcon icon="plus" className="h-4 w-4" />
                )}
                Add rule
              </Button>
            </form>
          </section>

          <section className="flex flex-col gap-3">
            <div>
              <h2 className="flex items-center gap-2 text-sm font-medium text-foreground">
                <AppIcon
                  icon="brain"
                  className="text-muted-foreground h-4 w-4"
                />
                Memories
              </h2>
              <p className="text-muted-foreground mt-1 text-sm">
                Built automatically from sessions in this channel. Confirming a
                memory strengthens it; deleting prevents re-learning.
              </p>
            </div>

            {observationsQuery.isLoading ? (
              <RowSkeletons count={3} />
            ) : observationsQuery.isError ? (
              <ErrorState label="Could not load memories" />
            ) : observations.length === 0 ? (
              <EmptyCard
                icon="brain"
                title="No memories yet"
                body="Memories appear here as the agent works — durable facts, decisions, and preferences it learns from sessions in this channel."
              />
            ) : (
              <div className="flex flex-col gap-2">
                {observations.map((observation) => (
                  <ObservationRow
                    key={observation.id}
                    observation={observation}
                    onConfirm={() => confirmMutation.mutate(observation.id)}
                    onEdit={() => setEditTarget(observation)}
                    onPin={() => pinMutation.mutate(observation.id)}
                    onDelete={() => setDeleteTarget(observation)}
                  />
                ))}
              </div>
            )}

            {hasMore ? (
              <button
                type="button"
                onClick={loadMore}
                disabled={loadingMore}
                className="text-muted-foreground flex items-center justify-center gap-2 self-start rounded-lg px-3 py-1.5 text-sm transition-colors hover:bg-default disabled:opacity-60"
              >
                {loadingMore ? (
                  <Spinner color="current" size="sm" />
                ) : (
                  <AppIcon icon="chevron-down" className="h-4 w-4" />
                )}
                Load more
              </button>
            ) : null}
          </section>
        </>
      )}

      <EditContentModal
        open={editTarget !== null}
        heading="Edit memory"
        description="Your correction becomes the verified version of this memory."
        initialContent={editTarget?.content ?? ""}
        pending={correctMutation.isPending}
        onOpenChange={(open) => {
          if (!open && !correctMutation.isPending) setEditTarget(null)
        }}
        onSave={(content) => {
          if (editTarget) correctMutation.mutate({ id: editTarget.id, content })
        }}
      />

      <DeleteRuleDialog
        target={deleteRuleTarget}
        pending={deleteRuleMutation.isPending}
        onOpenChange={(open) => {
          if (!open && !deleteRuleMutation.isPending) setDeleteRuleTarget(null)
        }}
        onConfirm={() => {
          if (deleteRuleTarget) deleteRuleMutation.mutate(deleteRuleTarget.id)
        }}
      />

      <DeleteMemoryDialog
        target={deleteTarget}
        pending={deleteMutation.isPending}
        onOpenChange={(open) => {
          if (!open && !deleteMutation.isPending) setDeleteTarget(null)
        }}
        onConfirm={() => {
          if (deleteTarget) deleteMutation.mutate(deleteTarget.id)
        }}
      />
    </div>
  )
}
