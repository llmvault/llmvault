"use client"

import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { Button, EmptyState, Skeleton, Spinner } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import {
  CompactSelection,
  type GridSelection,
} from "@glideapps/glide-data-grid"
import { extractErrorMessage } from "@/lib/api/error"
import {
  compileFilter,
  createSheetField,
  createView,
  deleteView,
  fetchViews,
  sheetKeys,
  updateView,
  type FilterRuleState,
  type SheetFieldSpec,
  type SheetPage,
  type SheetSort,
  type SheetView,
} from "@/app/w/(chat)/_lib/sheets"
import { SheetGrid } from "./grid"
import { ImportDialog } from "./import-dialog"
import { SheetToolbar } from "./toolbar"
import { useSheetRows } from "./use-sheet-rows"

const PERSIST_DEBOUNCE_MS = 800

const EMPTY_SELECTION: GridSelection = {
  columns: CompactSelection.empty(),
  rows: CompactSelection.empty(),
}

interface PersistedConfig {
  column_widths?: Record<string, number>
  filters?: FilterRuleState[]
  /** Ordered multi-sort. Older configs stored a single `sort` instead. */
  sorts?: SheetSort[]
  hidden_fields?: string[]
}

function parsePersistedConfig(raw: unknown): PersistedConfig {
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) return {}
  const record = raw as Record<string, unknown>
  const config: PersistedConfig = {}
  const widths = record.column_widths
  if (widths && typeof widths === "object" && !Array.isArray(widths)) {
    const parsed: Record<string, number> = {}
    for (const [key, value] of Object.entries(widths)) {
      if (typeof value === "number" && Number.isFinite(value)) {
        parsed[key] = value
      }
    }
    config.column_widths = parsed
  }
  if (Array.isArray(record.filters)) {
    config.filters = record.filters.filter(
      (entry): entry is FilterRuleState =>
        Boolean(entry) &&
        typeof entry === "object" &&
        typeof (entry as FilterRuleState).field === "string" &&
        typeof (entry as FilterRuleState).op === "string"
    )
  }
  const isSort = (entry: unknown): entry is SheetSort =>
    Boolean(entry) &&
    typeof entry === "object" &&
    typeof (entry as SheetSort).field === "string"
  if (Array.isArray(record.sorts)) {
    config.sorts = record.sorts.filter(isSort)
  } else if (isSort(record.sort)) {
    // Legacy single-sort config.
    config.sorts = [record.sort]
  }
  if (Array.isArray(record.hidden_fields)) {
    config.hidden_fields = record.hidden_fields.filter(
      (entry): entry is string => typeof entry === "string"
    )
  }
  return config
}

/** Placeholder rows shown while the first window of rows loads. */
function SkeletonRows() {
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
 * Everything for one sheet page: toolbar, Glide grid, import dialog.
 * Mount with key={pageId} so all local state resets per page. Waits for the
 * page's saved views so filters/sorts/widths initialize from the active
 * view's config; switching views remounts the inner workbench with the
 * selected view's config. When no view is selected the first (implicit
 * default) view applies, matching the previous behavior.
 */
export function SheetWorkbench({
  sheetId,
  channelId,
  page,
  pages,
}: {
  sheetId: string
  channelId: string
  page: SheetPage
  pages: SheetPage[]
}) {
  const pageId = page.id ?? ""
  const [selectedViewId, setSelectedViewId] = useState<string | null>(null)
  const viewsQuery = useQuery({
    queryKey: sheetKeys.views(pageId),
    queryFn: ({ signal }) => fetchViews(sheetId, pageId, signal),
  })

  if (viewsQuery.isPending) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner size="sm" />
      </div>
    )
  }

  const views = viewsQuery.data?.views ?? []
  const activeView =
    views.find((view) => view.id === selectedViewId) ?? views[0] ?? null
  return (
    <WorkbenchInner
      key={activeView?.id ?? "default"}
      sheetId={sheetId}
      channelId={channelId}
      page={page}
      pages={pages}
      views={views}
      activeViewId={activeView?.id ?? null}
      onSelectView={setSelectedViewId}
      initialConfig={parsePersistedConfig(activeView?.config)}
    />
  )
}

function WorkbenchInner({
  sheetId,
  channelId,
  page,
  pages,
  views,
  activeViewId,
  onSelectView,
  initialConfig,
}: {
  sheetId: string
  channelId: string
  page: SheetPage
  pages: SheetPage[]
  views: SheetView[]
  activeViewId: string | null
  onSelectView: (viewId: string | null) => void
  initialConfig: PersistedConfig
}) {
  const pageId = page.id ?? ""
  const fields = useMemo(() => page.fields ?? [], [page.fields])
  const queryClient = useQueryClient()

  const [columnWidths, setColumnWidths] = useState<Record<string, number>>(
    () => initialConfig.column_widths ?? {}
  )
  const [filterRules, setFilterRules] = useState<FilterRuleState[]>(
    () => initialConfig.filters ?? []
  )
  const [sorts, setSorts] = useState<SheetSort[]>(
    () => initialConfig.sorts ?? []
  )
  const [hiddenFieldIds, setHiddenFieldIds] = useState<string[]>(
    () => initialConfig.hidden_fields ?? []
  )
  // The raw search text and its debounce live inside the toolbar's SearchBox
  // leaf, so keystrokes don't re-render the whole workbench. We only hold the
  // committed (debounced) query here, plus a signal to imperatively clear it.
  const [searchQuery, setSearchQuery] = useState("")
  const [searchResetSignal, setSearchResetSignal] = useState(0)
  const [selection, setSelection] = useState<GridSelection>(EMPTY_SELECTION)
  const [importOpen, setImportOpen] = useState(false)

  /* ---------------- view config: debounce persist -------------------- */

  const creatingViewRef = useRef(false)
  const latestRef = useRef({
    columnWidths,
    filterRules,
    sorts,
    hiddenFieldIds,
    activeViewId,
  })
  useEffect(() => {
    latestRef.current = {
      columnWidths,
      filterRules,
      sorts,
      hiddenFieldIds,
      activeViewId,
    }
  })

  const currentConfig = useCallback(
    () => ({
      column_widths: latestRef.current.columnWidths,
      filters: latestRef.current.filterRules,
      sorts: latestRef.current.sorts,
      hidden_fields: latestRef.current.hiddenFieldIds,
    }),
    []
  )

  const persistTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const schedulePersist = useCallback(() => {
    if (persistTimerRef.current) clearTimeout(persistTimerRef.current)
    persistTimerRef.current = setTimeout(() => {
      const latest = latestRef.current
      const config = currentConfig()
      if (latest.activeViewId) {
        updateView(sheetId, pageId, latest.activeViewId, { config }).catch(
          () => {
            /* view persistence is best-effort */
          }
        )
      } else if (!creatingViewRef.current) {
        creatingViewRef.current = true
        createView(sheetId, pageId, { name: "Grid", type: "grid", config })
          .then(() => {
            void queryClient.invalidateQueries({
              queryKey: sheetKeys.views(pageId),
            })
          })
          .catch(() => {
            /* best-effort */
          })
          .finally(() => {
            creatingViewRef.current = false
          })
      }
    }, PERSIST_DEBOUNCE_MS)
  }, [currentConfig, pageId, queryClient, sheetId])

  useEffect(
    () => () => {
      if (persistTimerRef.current) clearTimeout(persistTimerRef.current)
    },
    []
  )

  /* ---------------- rows ------------------------------------------- */

  const filter = useMemo(
    () => compileFilter(filterRules, fields),
    [filterRules, fields]
  )

  const controller = useSheetRows({
    sheetId,
    pageId,
    filter,
    sorts: sorts.length > 0 ? sorts : undefined,
    search: searchQuery || undefined,
  })

  const selectedRowIds = useMemo(() => {
    const ids: string[] = []
    for (const index of selection.rows) {
      const id = controller.rows[index]?.id
      if (id) ids.push(id)
    }
    return ids
  }, [selection.rows, controller.rows])

  /* ---------------- handlers ---------------------------------------- */

  const onColumnWidthChange = useCallback(
    (fieldId: string, width: number) => {
      setColumnWidths((prev) => ({ ...prev, [fieldId]: width }))
      schedulePersist()
    },
    [schedulePersist]
  )

  const onFilterRulesChange = useCallback(
    (rules: FilterRuleState[]) => {
      setFilterRules(rules)
      schedulePersist()
    },
    [schedulePersist]
  )

  const onSortsChange = useCallback(
    (nextSorts: SheetSort[]) => {
      setSorts(nextSorts)
      schedulePersist()
    },
    [schedulePersist]
  )

  /* ---------------- named views -------------------------------------- */

  const onSaveAsView = useCallback(
    async (name: string) => {
      const created = await createView(sheetId, pageId, {
        name,
        type: "grid",
        config: currentConfig(),
      })
      await queryClient.invalidateQueries({ queryKey: sheetKeys.views(pageId) })
      if (created.id) onSelectView(created.id)
    },
    [currentConfig, onSelectView, pageId, queryClient, sheetId]
  )

  const onRenameView = useCallback(
    async (name: string) => {
      if (!activeViewId) return
      await updateView(sheetId, pageId, activeViewId, { name })
      await queryClient.invalidateQueries({ queryKey: sheetKeys.views(pageId) })
    },
    [activeViewId, pageId, queryClient, sheetId]
  )

  const onDeleteView = useCallback(async () => {
    if (!activeViewId) return
    await deleteView(sheetId, pageId, activeViewId)
    await queryClient.invalidateQueries({ queryKey: sheetKeys.views(pageId) })
    onSelectView(null)
  }, [activeViewId, onSelectView, pageId, queryClient, sheetId])

  const onHideField = useCallback(
    (fieldId: string) => {
      setHiddenFieldIds((prev) =>
        prev.includes(fieldId) ? prev : [...prev, fieldId]
      )
      schedulePersist()
    },
    [schedulePersist]
  )

  const onUnhideField = useCallback(
    (fieldId: string) => {
      setHiddenFieldIds((prev) => prev.filter((entry) => entry !== fieldId))
      schedulePersist()
    },
    [schedulePersist]
  )

  const visibleFields = useMemo(
    () => fields.filter((field) => !hiddenFieldIds.includes(field.id ?? "")),
    [fields, hiddenFieldIds]
  )
  const hiddenFields = useMemo(
    () => fields.filter((field) => hiddenFieldIds.includes(field.id ?? "")),
    [fields, hiddenFieldIds]
  )

  const onDeleteSelected = useCallback(() => {
    if (selectedRowIds.length === 0) return
    void controller.deleteRowIds(selectedRowIds)
    setSelection(EMPTY_SELECTION)
  }, [controller, selectedRowIds])

  const onAddField = useCallback(
    async (spec: SheetFieldSpec) => {
      await createSheetField(sheetId, pageId, spec)
      await queryClient.invalidateQueries({
        queryKey: sheetKeys.structure(sheetId),
      })
    },
    [pageId, queryClient, sheetId]
  )

  const onOpenImport = useCallback(() => setImportOpen(true), [])

  /* ---------------- render ------------------------------------------ */

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <SheetToolbar
        sheetId={sheetId}
        pageId={pageId}
        pages={pages}
        fields={fields}
        onSearchChange={setSearchQuery}
        searchResetSignal={searchResetSignal}
        filterRules={filterRules}
        onFilterRulesChange={onFilterRulesChange}
        sorts={sorts}
        onSortsChange={onSortsChange}
        views={views}
        activeViewId={activeViewId}
        onSelectView={onSelectView}
        onSaveAsView={onSaveAsView}
        onRenameView={onRenameView}
        onDeleteView={onDeleteView}
        selectedRowIds={selectedRowIds}
        onDeleteSelected={onDeleteSelected}
        onOpenImport={onOpenImport}
        onAddField={onAddField}
        hiddenFields={hiddenFields}
        onUnhideField={onUnhideField}
      />

      {fields.length === 0 ? (
        <EmptyState className="flex flex-1 flex-col items-center justify-center gap-2 p-6 text-center">
          <AppIcon icon="columns-3" className="h-6 w-6 text-muted" />
          <p className="text-sm text-muted">
            This page has no columns yet. Add one to start entering data.
          </p>
        </EmptyState>
      ) : controller.isPending ? (
        <SkeletonRows />
      ) : controller.isError ? (
        <div className="flex flex-1 flex-col items-center justify-center gap-3 p-6">
          <p className="text-sm text-muted">
            {extractErrorMessage(controller.error, "Could not load rows.")}
          </p>
          <Button
            size="sm"
            variant="secondary"
            onPress={() => controller.refetch()}
          >
            Retry
          </Button>
        </div>
      ) : controller.rows.length === 0 ? (
        filterRules.length > 0 || searchQuery ? (
          <EmptyState className="flex flex-1 flex-col items-center justify-center gap-2 p-6 text-center">
            <AppIcon icon="list-filter" className="h-6 w-6 text-muted" />
            <p className="text-sm font-medium text-foreground">
              No rows match
            </p>
            <p className="max-w-sm text-xs text-muted">
              Adjust or clear the filters and search to see rows again.
            </p>
            <Button
              size="sm"
              variant="secondary"
              className="mt-1"
              onPress={() => {
                onFilterRulesChange([])
                setSearchResetSignal((n) => n + 1)
              }}
            >
              Clear filters
            </Button>
          </EmptyState>
        ) : (
          <EmptyState className="flex flex-1 flex-col items-center justify-center gap-2 p-6 text-center">
            <AppIcon icon="rows-3" className="h-6 w-6 text-muted" />
            <p className="text-sm font-medium text-foreground">No rows yet</p>
            <p className="max-w-sm text-xs text-muted">
              Add a first row or import a CSV to fill this page.
            </p>
            <div className="mt-1 flex items-center gap-2">
              <Button
                size="sm"
                variant="secondary"
                onPress={() => void controller.appendRow()}
              >
                <AppIcon icon="plus" className="h-3.5 w-3.5" />
                Add row
              </Button>
              <Button
                size="sm"
                variant="ghost"
                onPress={() => setImportOpen(true)}
              >
                <AppIcon icon="file-up" className="h-3.5 w-3.5" />
                Import CSV
              </Button>
            </div>
          </EmptyState>
        )
      ) : (
        <SheetGrid
          sheetId={sheetId}
          pageId={pageId}
          channelId={channelId}
          fields={visibleFields}
          controller={controller}
          columnWidths={columnWidths}
          onColumnWidthChange={onColumnWidthChange}
          selection={selection}
          onSelectionChange={setSelection}
          onHideField={onHideField}
        />
      )}

      <ImportDialog
        isOpen={importOpen}
        onOpenChange={setImportOpen}
        sheetId={sheetId}
        pageId={pageId}
        fields={fields}
      />
    </div>
  )
}
