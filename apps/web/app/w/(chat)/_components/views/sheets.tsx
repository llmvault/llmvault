"use client"

import { useCallback, useEffect, useState } from "react"
import dynamic from "next/dynamic"
import {
  Button,
  EmptyState,
  Input,
  ListBox,
  Popover,
  Select,
  Skeleton,
  Spinner,
  toast,
} from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { extractErrorMessage } from "@/lib/api/error"
import {
  createSheet,
  createSheetPage,
  fetchSheets,
  fetchSheetStructure,
  sheetKeys,
} from "@/app/w/(chat)/_lib/sheets"
import { usePanelSheetTargetStore } from "@/app/w/(chat)/_stores/panel-sheet-target-store"
import { PageTab } from "./sheets/page-tab"
import { useSheetLive } from "./sheets/use-sheet-live"

/** Shimmer placeholder shown while a sheet's grid loads. */
function SheetGridSkeleton() {
  return (
    <div className="flex flex-1 flex-col gap-2 overflow-hidden p-3">
      <Skeleton className="h-7 w-full" />
      {Array.from({ length: 12 }).map((_, index) => (
        <Skeleton
          key={index}
          className="h-6 w-full rounded-md"
          style={{ opacity: Math.max(0.25, 1 - index * 0.07) }}
        />
      ))}
    </div>
  )
}

/**
 * Glide Data Grid is canvas-based and touches `window`/`document`, so the
 * whole workbench chunk (grid + editors) is loaded client-only.
 */
const SheetWorkbench = dynamic(
  () => import("./sheets/workbench").then((m) => m.SheetWorkbench),
  {
    ssr: false,
    loading: () => <SheetGridSkeleton />,
  }
)

export function SheetsView({ channelId }: { channelId?: string }) {
  const queryClient = useQueryClient()
  const [selectedSheetId, setSelectedSheetId] = useState<string | null>(null)

  // When there is no session channel (e.g. opened from the /w/sheets
  // dashboard), fall back to the launched sheet's channel + id.
  const target = usePanelSheetTargetStore((state) => state.target)
  const effectiveChannelId = channelId ?? target?.channelId
  useEffect(() => {
    if (!channelId && target?.sheetId) setSelectedSheetId(target.sheetId)
  }, [channelId, target?.sheetId])

  const sheetsQuery = useQuery({
    enabled: Boolean(effectiveChannelId),
    queryKey: sheetKeys.list(effectiveChannelId ?? ""),
    queryFn: ({ signal }) => fetchSheets(effectiveChannelId ?? "", signal),
  })
  // Newest-updated first from the API; default to the most recent sheet.
  const sheets = sheetsQuery.data?.sheets ?? []
  const activeSheetId =
    (selectedSheetId && sheets.some((sheet) => sheet.id === selectedSheetId)
      ? selectedSheetId
      : null) ??
    sheets[0]?.id ??
    null
  const activeSheet = sheets.find((sheet) => sheet.id === activeSheetId) ?? null

  const createNewSheet = async (name: string) => {
    if (!effectiveChannelId) return
    const created = await createSheet(effectiveChannelId, name)
    await queryClient.invalidateQueries({
      queryKey: sheetKeys.list(effectiveChannelId),
    })
    if (created.sheet?.id) {
      setSelectedSheetId(created.sheet.id)
    }
  }

  if (!effectiveChannelId) {
    return <SheetGridSkeleton />
  }

  if (sheetsQuery.isPending) {
    return <SheetGridSkeleton />
  }

  if (sheetsQuery.isError) {
    return (
      <SheetsState
        icon="circle-alert"
        title="Sheets are not available"
        message={extractErrorMessage(
          sheetsQuery.error,
          "Could not load sheets."
        )}
        action={
          <Button
            size="sm"
            variant="secondary"
            onPress={() => void sheetsQuery.refetch()}
          >
            Retry
          </Button>
        }
      />
    )
  }

  if (sheets.length === 0) {
    return (
      <SheetsState
        icon="table"
        title="No sheets yet"
        message="Create a sheet, or ask an agent with the Sheets plugin to build one for you."
        action={<NamePopover label="New sheet" onSubmit={createNewSheet} />}
      />
    )
  }

  return (
    <div className="flex h-full min-h-0 flex-col bg-background">
      <div className="flex shrink-0 items-center gap-1.5 border-b border-border px-2 py-1.5">
        <Select
          aria-label="Sheet"
          selectedKey={activeSheetId}
          onSelectionChange={(key) => {
            if (key === null) return
            setSelectedSheetId(String(key))
          }}
          className="min-w-0"
        >
          <Select.Trigger className="flex h-8 max-w-80 items-center gap-1.5 py-1 pl-2 pe-7 text-sm">
            <AppIcon
              icon="table"
              className="h-3.5 w-3.5 shrink-0 text-muted"
            />
            <span className="min-w-0 flex-1 truncate text-foreground">
              {activeSheet?.name ?? "Select sheet"}
            </span>
            <Select.Indicator />
          </Select.Trigger>
          <Select.Popover className="w-72 p-1.5">
            <ListBox>
              {sheets.map((sheet) => (
                <ListBox.Item
                  key={sheet.id}
                  id={sheet.id ?? ""}
                  textValue={sheet.name ?? ""}
                >
                  <span className="flex min-w-0 items-center gap-2">
                    <AppIcon
                      icon="table"
                      className="h-3.5 w-3.5 shrink-0 text-muted"
                    />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm text-foreground">
                        {sheet.name}
                      </span>
                      {sheet.description ? (
                        <span className="block truncate text-xs text-muted">
                          {sheet.description}
                        </span>
                      ) : null}
                    </span>
                  </span>
                </ListBox.Item>
              ))}
            </ListBox>
          </Select.Popover>
        </Select>

        <NamePopover label="New sheet" iconOnly onSubmit={createNewSheet} />

        <div className="flex-1" />

        <Button
          variant="ghost"
          size="sm"
          isIconOnly
          aria-label="Refresh"
          onPress={() => void sheetsQuery.refetch()}
        >
          <AppIcon icon="refresh-cw" className="h-3.5 w-3.5 text-muted" />
        </Button>
      </div>

      {activeSheetId ? (
        <SheetPanel
          key={activeSheetId}
          channelId={effectiveChannelId}
          sheetId={activeSheetId}
        />
      ) : null}
    </div>
  )
}

/**
 * Renders a single sheet — its pages, live grid, and page tabs — for a known
 * sheet id. Session-independent: it needs only the channel the sheet belongs
 * to (for grid mutations) and the sheet id. Reused by SheetsView (session
 * chat panel) and by the standalone /w/sheets dashboard right panel.
 */
function SheetPanel({
  channelId,
  sheetId,
}: {
  channelId: string
  sheetId: string
}) {
  const queryClient = useQueryClient()
  const [selectedPageId, setSelectedPageId] = useState<string | null>(null)
  // Stable across renders so memoized PageTabs don't re-render on every parent
  // update (e.g. live structure refetches).
  const handleSelectPage = useCallback(
    (pageId: string | null) => setSelectedPageId(pageId),
    []
  )

  const structureQuery = useQuery({
    enabled: Boolean(sheetId),
    queryKey: sheetKeys.structure(sheetId),
    queryFn: ({ signal }) => fetchSheetStructure(sheetId, signal),
  })
  const pages = structureQuery.data?.pages ?? []
  const activePage =
    pages.find((page) => page.id === selectedPageId) ?? pages[0] ?? null

  useSheetLive(sheetId, activePage?.id ?? null)

  const createNewPage = async (name: string) => {
    const created = await createSheetPage(sheetId, name)
    await queryClient.invalidateQueries({
      queryKey: sheetKeys.structure(sheetId),
    })
    if (created.id) setSelectedPageId(created.id)
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col bg-background">
      {structureQuery.isPending ? (
        <SheetGridSkeleton />
      ) : structureQuery.isError ? (
        <SheetsState
          icon="circle-alert"
          title="Could not load this sheet"
          message={extractErrorMessage(
            structureQuery.error,
            "Could not load the sheet structure."
          )}
          action={
            <Button
              size="sm"
              variant="secondary"
              onPress={() => void structureQuery.refetch()}
            >
              Retry
            </Button>
          }
        />
      ) : activePage?.id ? (
        <SheetWorkbench
          key={activePage.id}
          sheetId={sheetId}
          channelId={channelId}
          page={activePage}
          pages={pages}
        />
      ) : (
        <SheetsState
          icon="table"
          title="No pages"
          message="Add a page to start entering data, or import a CSV once one exists."
          action={<NamePopover label="New page" onSubmit={createNewPage} />}
        />
      )}

      <div className="flex shrink-0 items-center gap-1 overflow-x-auto border-t border-border px-2 py-1">
        {pages.map((page) => (
          <PageTab
            key={page.id}
            sheetId={sheetId}
            page={page}
            pages={pages}
            isActive={page.id === activePage?.id}
            onSelect={handleSelectPage}
          />
        ))}
        <NamePopover label="New page" iconOnly onSubmit={createNewPage} />
      </div>
    </div>
  )
}

function SheetsState({
  icon,
  title,
  message,
  action,
}: {
  icon: string
  title: string
  message?: string
  action?: React.ReactNode
}) {
  return (
    <EmptyState className="flex h-full flex-1 flex-col items-center justify-center gap-2 bg-background p-6 text-center">
      <AppIcon icon={icon} className="h-6 w-6 text-muted" />
      <p className="text-sm font-medium text-foreground">{title}</p>
      {message ? <p className="max-w-sm text-xs text-muted">{message}</p> : null}
      {action ? <div className="mt-1">{action}</div> : null}
    </EmptyState>
  )
}

function NamePopover({
  label,
  iconOnly = false,
  onSubmit,
}: {
  label: string
  iconOnly?: boolean
  onSubmit: (name: string) => Promise<void>
}) {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState("")
  const [saving, setSaving] = useState(false)

  const submit = async () => {
    const trimmed = name.trim()
    if (!trimmed || saving) return
    setSaving(true)
    try {
      await onSubmit(trimmed)
      setName("")
      setOpen(false)
    } catch (error) {
      toast.danger(extractErrorMessage(error, `Could not create: ${label}`))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Popover isOpen={open} onOpenChange={setOpen}>
      <Popover.Trigger
        aria-label={label}
        className={`flex shrink-0 items-center gap-1.5 rounded-lg text-muted transition-colors hover:bg-default ${
          iconOnly ? "p-1.5" : "border border-border px-2.5 py-1.5 text-sm"
        }`}
      >
        <AppIcon icon="plus" className="h-3.5 w-3.5" />
        {iconOnly ? null : label}
      </Popover.Trigger>
      <Popover.Content className="w-64 border border-border p-3">
        <Popover.Dialog className="flex w-full flex-col gap-2 p-0">
          <Input
            aria-label={`${label} name`}
            placeholder="Name"
            value={name}
            onChange={(event) => setName(event.target.value)}
            autoFocus
            onKeyDown={(event) => {
              if (event.key === "Enter") void submit()
            }}
          />
          <Button
            size="sm"
            isDisabled={!name.trim() || saving}
            onPress={() => void submit()}
          >
            {saving ? <Spinner size="sm" /> : null}
            {label}
          </Button>
        </Popover.Dialog>
      </Popover.Content>
    </Popover>
  )
}
