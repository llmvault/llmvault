"use client"

import { useEffect, useMemo, useState, type ComponentProps } from "react"
import Link from "next/link"
import { useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Popover, Skeleton, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { useAuth } from "@/lib/auth/auth-context"
import { useIsAdmin } from "@/lib/auth/use-role"
import { cn } from "@/lib/utils"
import { ProviderIcon } from "./_provider-icon"
import { TutorialBanner } from "@/components/tutorial-banner"
import {
  deriveProgress,
  deriveProvider,
  deriveStatus,
  providerMeta,
  RAG_SOURCES_QUERY_KEY,
  scopeSummary,
  type Connection,
  type DerivedProgress,
  type RagSource,
  type SourceStatus,
} from "./_lib"

const STATUS_META: Record<SourceStatus, { icon: string; className: string }> = {
  syncing: { icon: "loader-circle", className: "text-primary" },
  active: { icon: "circle-check", className: "text-foreground" },
  paused: { icon: "circle-slash", className: "text-muted-foreground" },
  disabled: { icon: "eye-off", className: "text-muted-foreground" },
  error: { icon: "circle-alert", className: "text-danger" },
}

const EMPTY_SOURCES: RagSource[] = []
const EMPTY_CONNECTIONS: Connection[] = []

export default function KnowledgeSettingsPage() {
  const router = useRouter()
  const queryClient = useQueryClient()
  const isAdmin = useIsAdmin()
  const { isLoading: authLoading } = useAuth()
  const [removeTarget, setRemoveTarget] = useState<{ id: string; name: string } | null>(null)

  const sourcesQuery = $api.useQuery(
    "get",
    "/v1/rag/sources",
    { params: { query: { page_size: 100 } } },
    {
      enabled: isAdmin,
      refetchInterval: (query) => {
        const rows = query.state.data?.data ?? []
        return rows.some((s) => deriveStatus(s) === "syncing") ? 5000 : false
      },
    }
  )
  const connectionsQuery = $api.useQuery(
    "get",
    "/v1/connections",
    { params: { query: { limit: 100 } } },
    { enabled: isAdmin }
  )

  useEffect(() => {
    if (!authLoading && !isAdmin) router.replace("/w/settings/teams")
  }, [authLoading, isAdmin, router])

  const sources = sourcesQuery.data?.data ?? EMPTY_SOURCES
  const connections = connectionsQuery.data?.data ?? EMPTY_CONNECTIONS
  const connectionsById = useMemo(() => {
    const map = new Map<string, Connection>()
    for (const conn of connections) if (conn.id) map.set(conn.id, conn)
    return map
  }, [connections])

  const invalidateSources = () =>
    queryClient.invalidateQueries({ queryKey: RAG_SOURCES_QUERY_KEY })

  const updateSource = $api.useMutation("patch", "/v1/rag/sources/{id}", {
    onSuccess: invalidateSources,
    onError: (error) => toast.danger(extractErrorMessage(error, "Could not update source")),
  })
  const deleteSource = $api.useMutation("delete", "/v1/rag/sources/{id}", {
    onSuccess: invalidateSources,
    onError: (error) => toast.danger(extractErrorMessage(error, "Could not remove source")),
  })

  function patchSource(id: string, name: string, body: Record<string, unknown>, ok: string) {
    updateSource.mutate(
      { params: { path: { id } }, body },
      { onSuccess: () => toast.success(ok.replace("{name}", name)) }
    )
  }

  function confirmRemove() {
    if (!removeTarget) return
    const { name } = removeTarget
    deleteSource.mutate(
      { params: { path: { id: removeTarget.id } } },
      {
        onSuccess: () => {
          toast.success(`Removed ${name}`)
          setRemoveTarget(null)
        },
      }
    )
  }

  if (!isAdmin) return null

  return (
    <div className="flex flex-col gap-8">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-lg font-semibold text-foreground">Knowledge</h1>
          <p className="mt-1 max-w-lg text-sm text-muted-foreground">
            Connect sources so your agents can search company knowledge. Each
            source ingests a scope you choose and is available to its teams.
          </p>
        </div>
        {isAdmin ? (
          <Button
            variant="primary"
            size="sm"
            className="shrink-0"
            onPress={() => router.push("/w/settings/knowledge/new")}
          >
            <AppIcon icon="plus" className="h-4 w-4" />
            Add source
          </Button>
        ) : null}
      </div>

      <TutorialBanner
        tutorial="knowledge"
        title="Give agents trusted company context"
        description="See how to connect a source, choose its scope, and grant team access."
        docsPath="knowledge-and-memory/knowledge-sources"
      />

      {sourcesQuery.isLoading ? (
        <SourcesSkeleton />
      ) : sourcesQuery.isError ? (
        <ErrorState onRetry={() => sourcesQuery.refetch()} />
      ) : sources.length === 0 ? (
        <EmptyState isAdmin={isAdmin} />
      ) : (
        <div className="flex flex-col gap-2">
          {sources.map((source) => (
            <SourceCard
              key={source.id}
              source={source}
              connectionsById={connectionsById}
              isAdmin={isAdmin}
              onRemove={() => setRemoveTarget({ id: source.id!, name: source.name ?? "source" })}
              onPatch={(body, ok) => patchSource(source.id!, source.name ?? "source", body, ok)}
            />
          ))}
        </div>
      )}

      <ConfirmDialog
        open={removeTarget !== null}
        pending={deleteSource.isPending}
        heading="Remove knowledge source?"
        description={
          removeTarget
            ? `“${removeTarget.name}” and its indexed documents will be removed. Agents will no longer be able to search it. This can’t be undone.`
            : ""
        }
        confirmLabel="Remove source"
        onOpenChange={(open) => {
          if (!open) setRemoveTarget(null)
        }}
        onConfirm={confirmRemove}
      />
    </div>
  )
}

function SourceCard({
  source,
  connectionsById,
  isAdmin,
  onRemove,
  onPatch,
}: {
  source: RagSource
  connectionsById: Map<string, Connection>
  isAdmin: boolean
  onRemove: () => void
  onPatch: (body: Record<string, unknown>, ok: string) => void
}) {
  const provider = providerMeta(deriveProvider(source, connectionsById))
  const status = deriveStatus(source)
  const statusMeta = STATUS_META[status]
  const progress = deriveProgress(source, status)

  return (
    <div className="flex items-start gap-3 rounded-xl border border-border bg-surface px-4 py-3.5">
      <div className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-default">
        <ProviderIcon icon={provider.icon} className="h-5 w-5" />
      </div>

      <div className="flex min-w-0 flex-1 flex-col gap-2">
        <div className="flex min-w-0 flex-col gap-0.5">
          <div className="flex items-center gap-2">
            <span className="truncate text-sm font-medium text-foreground">
              {source.name}
            </span>
            <span className="shrink-0 text-xs text-muted-foreground">
              {provider.label}
            </span>
          </div>
          <div className="text-xs text-muted-foreground">
            <span className="truncate">{scopeSummary(source, provider)}</span>
          </div>
        </div>

        <ProgressRow progress={progress} statusMeta={statusMeta} syncing={status === "syncing"} />
      </div>

      <SourceActionsMenu
        sourceId={source.id!}
        status={status}
        isAdmin={isAdmin}
        onRemove={onRemove}
        onPatch={onPatch}
      />
    </div>
  )
}

function ProgressRow({
  progress,
  statusMeta,
  syncing,
}: {
  progress: DerivedProgress
  statusMeta: { icon: string; className: string }
  syncing: boolean
}) {
  return (
    <div className="flex items-center gap-1.5">
      <AppIcon
        icon={statusMeta.icon}
        className={cn("h-3.5 w-3.5", statusMeta.className, syncing && "animate-spin")}
      />
      <span className={cn("text-xs", statusMeta.className)}>{progress.label}</span>
    </div>
  )
}

function SourceActionsMenu({
  sourceId,
  status,
  isAdmin,
  onRemove,
  onPatch,
  placement = "bottom end",
}: {
  sourceId: string
  status: SourceStatus
  isAdmin: boolean
  onRemove: () => void
  onPatch: (body: Record<string, unknown>, ok: string) => void
  placement?: ComponentProps<typeof Popover.Content>["placement"]
}) {
  const [open, setOpen] = useState(false)
  const disabled = status === "disabled"
  const paused = status === "paused"

  function act(fn: () => void) {
    fn()
    setOpen(false)
  }

  return (
    <Popover isOpen={open} onOpenChange={setOpen}>
      <Popover.Trigger
        aria-label="Source options"
        data-open={open ? "true" : undefined}
        className="hover:bg-default data-[open=true]:bg-default -mr-1 flex shrink-0 items-center rounded-md p-1 text-muted-foreground transition-colors"
      >
        <AppIcon icon="ellipsis" className="h-4 w-4" />
      </Popover.Trigger>
      {open ? (
        <Popover.Content
          placement={placement}
          offset={6}
          className="w-56 rounded-2xl border border-border p-1.5"
        >
          <Popover.Dialog className="flex w-full flex-col gap-0.5 p-0">
            {isAdmin ? (
              <MenuLink href={`/w/settings/knowledge/${sourceId}/edit`}>
                Edit source
              </MenuLink>
            ) : null}
            <MenuLink href={`/w/settings/knowledge/${sourceId}/documents`}>
              View documents
            </MenuLink>

            {isAdmin ? (
              <>
                {paused ? (
                  <MenuButton onClick={() => act(() => onPatch({ status: "ACTIVE" }, "Resumed {name}"))}>
                    Resume ingestion
                  </MenuButton>
                ) : (
                  <MenuButton onClick={() => act(() => onPatch({ status: "PAUSED" }, "Paused {name}"))}>
                    Pause ingestion
                  </MenuButton>
                )}

                {disabled ? (
                  <MenuButton onClick={() => act(() => onPatch({ enabled: true }, "Enabled {name}"))}>
                    Enable source
                  </MenuButton>
                ) : (
                  <MenuButton onClick={() => act(() => onPatch({ enabled: false }, "Disabled {name}"))}>
                    Disable source
                  </MenuButton>
                )}

                <div className="my-1 h-px bg-border" />

                <MenuButton danger onClick={() => act(onRemove)}>
                  Remove source
                </MenuButton>
              </>
            ) : null}
          </Popover.Dialog>
        </Popover.Content>
      ) : null}
    </Popover>
  )
}

function MenuButton({
  children,
  onClick,
  danger,
}: {
  children: React.ReactNode
  onClick: () => void
  danger?: boolean
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "flex items-center rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors",
        danger ? "text-danger hover:bg-danger/10" : "hover:bg-default text-foreground"
      )}
    >
      {children}
    </button>
  )
}

function MenuLink({ href, children }: { href: string; children: React.ReactNode }) {
  return (
    <Link
      href={href}
      className="hover:bg-default flex items-center rounded-xl px-2.5 py-1.5 text-left text-sm text-foreground transition-colors"
    >
      {children}
    </Link>
  )
}

function SourcesSkeleton() {
  return (
    <div className="flex flex-col gap-2">
      {[0, 1, 2].map((i) => (
        <div key={i} className="flex items-start gap-3 rounded-xl border border-border bg-surface px-4 py-3.5">
          <Skeleton className="h-9 w-9 rounded-lg" />
          <div className="flex flex-1 flex-col gap-2">
            <Skeleton className="h-4 w-40 rounded" />
            <Skeleton className="h-3 w-24 rounded" />
          </div>
        </div>
      ))}
    </div>
  )
}

function ErrorState({ onRetry }: { onRetry: () => void }) {
  return (
    <div className="flex min-h-56 flex-col items-center justify-center rounded-xl bg-card px-6 text-center">
      <AppIcon icon="circle-alert" className="h-7 w-7 text-danger" />
      <p className="mt-3 text-sm font-medium text-foreground">Couldn’t load sources</p>
      <p className="mt-1 max-w-sm text-sm text-muted-foreground">
        Something went wrong fetching your knowledge sources.
      </p>
      <Button variant="tertiary" size="sm" className="mt-4" onPress={onRetry}>
        Try again
      </Button>
    </div>
  )
}

function EmptyState({ isAdmin }: { isAdmin: boolean }) {
  return (
    <div className="flex min-h-56 flex-col items-center justify-center rounded-xl bg-card px-6 text-center">
      <AppIcon icon="folder-open" className="h-7 w-7 text-muted-foreground" />
      <p className="mt-3 text-sm font-medium text-foreground">No knowledge sources yet</p>
      <p className="mt-1 max-w-sm text-sm text-muted-foreground">
        {isAdmin
          ? "Add a source to let your agents search company knowledge from GitHub, Notion, Slack, or your website."
          : "A workspace admin can add sources so your agents can search company knowledge."}
      </p>
      {isAdmin ? (
        <Link
          href="/w/settings/knowledge/new"
          className="mt-4 inline-flex items-center gap-2 text-sm font-medium text-primary transition-colors hover:text-primary/80"
        >
          <AppIcon icon="plus" className="h-4 w-4" />
          Add source
        </Link>
      ) : null}
    </div>
  )
}
