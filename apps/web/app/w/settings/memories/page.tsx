"use client"

import { useMemo, useState, type FormEvent } from "react"
import {
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query"
import {
  AlertDialog,
  Button,
  Input,
  ListBox,
  Modal,
  Popover,
  Select,
  Skeleton,
  Spinner,
  TextArea,
  toast,
} from "@heroui/react"
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
  observationKind,
  pinObservation,
  updateDirective,
  type Directive,
  type Observation,
} from "@/lib/api/memory"

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
        <p className="mt-1 text-sm text-muted-foreground">
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
                  className="h-4 w-4 text-muted-foreground"
                />
                Rules
                {directives.length > 0 ? (
                  <span className="text-xs font-normal text-muted-foreground">
                    {directives.length}
                  </span>
                ) : null}
              </h2>
              <p className="mt-1 text-sm text-muted-foreground">
                Rules are injected into every session in this channel.
              </p>
            </div>

            {directivesQuery.isLoading ? (
              <RowSkeletons count={2} />
            ) : directivesQuery.isError ? (
              <ErrorState label="Could not load rules" />
            ) : directives.length === 0 ? (
              <div className="rounded-xl border border-dashed border-border px-4 py-5 text-center text-sm text-muted-foreground">
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
                  className="h-4 w-4 text-muted-foreground"
                />
                Memories
              </h2>
              <p className="mt-1 text-sm text-muted-foreground">
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
                className="hover:bg-default flex items-center justify-center gap-2 self-start rounded-lg px-3 py-1.5 text-sm text-muted-foreground transition-colors disabled:opacity-60"
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

function DirectiveRow({
  directive,
  onDeactivate,
  onDelete,
}: {
  directive: Directive
  onDeactivate: () => void
  onDelete: () => void
}) {
  return (
    <div className="group flex items-start gap-3 rounded-xl border border-border bg-surface px-4 py-3">
      <div className="flex min-w-0 flex-1 flex-col gap-2">
        <p className="text-sm leading-5 text-foreground">{directive.content}</p>
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="rounded-md bg-default px-1.5 py-0.5 text-xs text-muted-foreground">
            {directive.source === "user-pinned"
              ? "Pinned by you"
              : "From memory"}
          </span>
          {directive.createdAt ? (
            <span className="text-xs text-muted-foreground">
              · {relativeTime(directive.createdAt)}
            </span>
          ) : null}
        </div>
      </div>
      <ActionsMenu
        label="Rule options"
        items={[
          {
            key: "deactivate",
            label: "Deactivate rule",
            icon: "circle-slash",
            onSelect: onDeactivate,
          },
          {
            key: "delete",
            label: "Delete rule",
            icon: "trash-2",
            danger: true,
            onSelect: onDelete,
          },
        ]}
      />
    </div>
  )
}

function ObservationRow({
  observation,
  onConfirm,
  onEdit,
  onPin,
  onDelete,
}: {
  observation: Observation
  onConfirm: () => void
  onEdit: () => void
  onPin: () => void
  onDelete: () => void
}) {
  const kind = observationKind(observation.kind)
  return (
    <div className="group flex items-start gap-3 rounded-xl border border-border bg-surface px-4 py-3">
      <div className="flex min-w-0 flex-1 flex-col gap-2">
        <p className="text-sm leading-5 text-foreground">
          {observation.content}
        </p>
        <div className="flex flex-wrap items-center gap-1.5">
          <span
            className={`rounded-md px-1.5 py-0.5 text-xs font-medium ${kind.chipClass}`}
          >
            {kind.label}
          </span>
          {observation.humanVerified ? (
            <span className="flex items-center gap-1 rounded-md bg-success/15 px-1.5 py-0.5 text-xs text-success">
              <AppIcon icon="shield-check" className="h-3 w-3" />
              Verified
            </span>
          ) : null}
          {observation.proofCount > 1 ? (
            <span className="text-xs text-muted-foreground">
              confirmed {observation.proofCount}×
            </span>
          ) : null}
          {observation.entities.map((entity) => (
            <span
              key={entity}
              className="rounded-md bg-default px-1.5 py-0.5 text-xs text-muted-foreground"
            >
              {entity}
            </span>
          ))}
          {observation.expiresAt ? (
            <span className="flex items-center gap-1 text-xs text-warning">
              <AppIcon icon="clock-alert" className="h-3 w-3" />
              Expires {shortDate(observation.expiresAt)}
            </span>
          ) : null}
          <span className="text-xs text-muted-foreground">
            · {relativeTime(observation.lastMentionedAt || observation.createdAt)}
          </span>
        </div>
      </div>
      <ActionsMenu
        label="Memory options"
        items={[
          {
            key: "confirm",
            label: "Confirm",
            icon: "check-circle",
            onSelect: onConfirm,
          },
          { key: "edit", label: "Edit", icon: "pencil", onSelect: onEdit },
          {
            key: "pin",
            label: "Pin as rule",
            icon: "list-checks",
            onSelect: onPin,
          },
          {
            key: "delete",
            label: "Delete",
            icon: "trash-2",
            danger: true,
            onSelect: onDelete,
          },
        ]}
      />
    </div>
  )
}

type MenuItem = {
  key: string
  label: string
  icon: string
  danger?: boolean
  onSelect: () => void
}

function ActionsMenu({ label, items }: { label: string; items: MenuItem[] }) {
  const [open, setOpen] = useState(false)
  return (
    <Popover isOpen={open} onOpenChange={setOpen}>
      <Popover.Trigger
        aria-label={label}
        data-open={open ? "true" : undefined}
        className="hover:bg-default data-[open=true]:bg-default -mr-1 flex shrink-0 items-center rounded-md p-1 text-muted-foreground transition-colors"
      >
        <AppIcon icon="ellipsis" className="h-4 w-4" />
      </Popover.Trigger>
      {open ? (
        <Popover.Content
          placement="bottom end"
          offset={6}
          className="w-52 rounded-2xl border border-border p-1.5"
        >
          <Popover.Dialog className="flex w-full flex-col gap-0.5 p-0">
            {items.map((item) => (
              <button
                key={item.key}
                type="button"
                onClick={() => {
                  item.onSelect()
                  setOpen(false)
                }}
                className={
                  item.danger
                    ? "flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-sm text-danger transition-colors hover:bg-danger/10"
                    : "hover:bg-default flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors"
                }
              >
                <AppIcon icon={item.icon} className="h-4 w-4 shrink-0" />
                {item.label}
              </button>
            ))}
          </Popover.Dialog>
        </Popover.Content>
      ) : null}
    </Popover>
  )
}

function EditContentModal({
  open,
  heading,
  description,
  initialContent,
  pending,
  onOpenChange,
  onSave,
}: {
  open: boolean
  heading: string
  description: string
  initialContent: string
  pending: boolean
  onOpenChange: (open: boolean) => void
  onSave: (content: string) => void
}) {
  if (!open) return null
  return (
    <EditContentModalContent
      heading={heading}
      description={description}
      initialContent={initialContent}
      pending={pending}
      onOpenChange={onOpenChange}
      onSave={onSave}
    />
  )
}

function EditContentModalContent({
  heading,
  description,
  initialContent,
  pending,
  onOpenChange,
  onSave,
}: {
  heading: string
  description: string
  initialContent: string
  pending: boolean
  onOpenChange: (open: boolean) => void
  onSave: (content: string) => void
}) {
  const [content, setContent] = useState(initialContent)
  const trimmed = content.trim()
  const invalid = trimmed.length === 0
  const unchanged = trimmed === initialContent.trim()

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (invalid || pending) return
    if (unchanged) {
      onOpenChange(false)
      return
    }
    onSave(trimmed)
  }

  return (
    <Modal isOpen onOpenChange={onOpenChange}>
      <Modal.Backdrop className="bg-background/80 backdrop-blur-sm">
        <Modal.Container placement="center" size="sm">
          <Modal.Dialog className="p-8">
            <Modal.CloseTrigger />
            <form onSubmit={submit}>
              <Modal.Header>
                <Modal.Icon className="bg-default size-12 text-foreground">
                  <AppIcon icon="pencil" className="h-6 w-6" />
                </Modal.Icon>
                <div className="flex flex-col gap-1">
                  <Modal.Heading>{heading}</Modal.Heading>
                  <p className="text-sm text-muted">{description}</p>
                </div>
              </Modal.Header>
              <Modal.Body>
                <TextArea
                  autoFocus
                  value={content}
                  disabled={pending}
                  aria-label={heading}
                  rows={4}
                  fullWidth
                  onChange={(event) => setContent(event.target.value)}
                />
              </Modal.Body>
              <Modal.Footer>
                <Button
                  type="button"
                  variant="tertiary"
                  size="sm"
                  isDisabled={pending}
                  onPress={() => onOpenChange(false)}
                >
                  Cancel
                </Button>
                <Button
                  type="submit"
                  variant="primary"
                  size="sm"
                  isDisabled={invalid || unchanged || pending}
                >
                  {pending ? <Spinner color="current" size="sm" /> : null}
                  Save
                </Button>
              </Modal.Footer>
            </form>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}

function DeleteRuleDialog({
  target,
  pending,
  onOpenChange,
  onConfirm,
}: {
  target: Directive | null
  pending: boolean
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
}) {
  return (
    <AlertDialog>
      <AlertDialog.Backdrop
        isOpen={target !== null}
        onOpenChange={(next) => {
          if (!pending) onOpenChange(next)
        }}
        className="bg-background/80 backdrop-blur-sm"
      >
        <AlertDialog.Container placement="center" size="sm">
          <AlertDialog.Dialog className="p-8">
            <AlertDialog.Header>
              <AlertDialog.Icon status="danger">
                <AppIcon icon="trash-2" className="h-6 w-6" />
              </AlertDialog.Icon>
              <div className="flex flex-col gap-1">
                <AlertDialog.Heading>Delete rule</AlertDialog.Heading>
                <p className="text-sm text-muted">
                  Rules can&apos;t be edited. To change this rule, delete it and
                  add a new one.
                </p>
              </div>
            </AlertDialog.Header>
            <AlertDialog.Footer>
              <Button
                variant="tertiary"
                size="sm"
                isDisabled={pending}
                onPress={() => onOpenChange(false)}
              >
                Cancel
              </Button>
              <Button
                variant="primary"
                size="sm"
                className="bg-danger text-danger-foreground hover:bg-danger/90"
                isDisabled={pending}
                onPress={onConfirm}
              >
                {pending ? <Spinner color="current" size="sm" /> : null}
                Delete
              </Button>
            </AlertDialog.Footer>
          </AlertDialog.Dialog>
        </AlertDialog.Container>
      </AlertDialog.Backdrop>
    </AlertDialog>
  )
}

function DeleteMemoryDialog({
  target,
  pending,
  onOpenChange,
  onConfirm,
}: {
  target: Observation | null
  pending: boolean
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
}) {
  return (
    <AlertDialog>
      <AlertDialog.Backdrop
        isOpen={target !== null}
        onOpenChange={(next) => {
          if (!pending) onOpenChange(next)
        }}
        className="bg-background/80 backdrop-blur-sm"
      >
        <AlertDialog.Container placement="center" size="sm">
          <AlertDialog.Dialog className="p-8">
            <AlertDialog.Header>
              <AlertDialog.Icon status="danger">
                <AppIcon icon="trash-2" className="h-6 w-6" />
              </AlertDialog.Icon>
              <div className="flex flex-col gap-1">
                <AlertDialog.Heading>Delete memory</AlertDialog.Heading>
                <p className="text-sm text-muted">
                  This removes the memory and prevents the agent from
                  re-learning it in this channel.
                </p>
              </div>
            </AlertDialog.Header>
            <AlertDialog.Footer>
              <Button
                variant="tertiary"
                size="sm"
                isDisabled={pending}
                onPress={() => onOpenChange(false)}
              >
                Cancel
              </Button>
              <Button
                variant="primary"
                size="sm"
                className="bg-danger text-danger-foreground hover:bg-danger/90"
                isDisabled={pending}
                onPress={onConfirm}
              >
                {pending ? <Spinner color="current" size="sm" /> : null}
                Delete
              </Button>
            </AlertDialog.Footer>
          </AlertDialog.Dialog>
        </AlertDialog.Container>
      </AlertDialog.Backdrop>
    </AlertDialog>
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
  const selected = options.find((option) => option.id === value)
  return (
    <Select
      aria-label="Channel"
      selectedKey={value || null}
      onSelectionChange={(key) => onChange(key === null ? "" : String(key))}
      className="w-full sm:w-64"
    >
      <Select.Trigger className="h-10 w-full justify-between px-3 text-sm transition-colors">
        <span className="flex min-w-0 items-center gap-2">
          <AppIcon
            icon="hash"
            className="h-4 w-4 shrink-0 text-muted-foreground"
          />
          <span className="truncate">{selected?.label ?? "Select channel"}</span>
        </span>
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover className="w-64 p-1.5">
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

function RowSkeletons({ count }: { count: number }) {
  return (
    <div className="flex flex-col gap-2">
      {Array.from({ length: count }).map((_, row) => (
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
  )
}

function LoadingState() {
  return (
    <div className="flex flex-col gap-8">
      <Skeleton className="h-10 w-64 rounded-lg" />
      <RowSkeletons count={2} />
      <RowSkeletons count={3} />
    </div>
  )
}

function ErrorState({ label }: { label: string }) {
  return (
    <div className="flex min-h-32 flex-col items-center justify-center rounded-xl bg-card px-6 text-center">
      <AppIcon icon="triangle-alert" className="h-7 w-7 text-muted-foreground" />
      <p className="mt-3 text-sm font-medium text-foreground">{label}</p>
      <p className="mt-1 max-w-sm text-sm text-muted-foreground">
        Refresh the page to try again.
      </p>
    </div>
  )
}

function EmptyCard({
  icon,
  title,
  body,
}: {
  icon: string
  title: string
  body: string
}) {
  return (
    <div className="flex min-h-56 flex-col items-center justify-center rounded-xl bg-card px-6 text-center">
      <AppIcon icon={icon} className="h-7 w-7 text-muted-foreground" />
      <p className="mt-3 text-sm font-medium text-foreground">{title}</p>
      <p className="mt-1 max-w-sm text-sm text-muted-foreground">{body}</p>
    </div>
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

function shortDate(iso: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ""
  return date.toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
  })
}
