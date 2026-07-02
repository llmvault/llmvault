"use client"

import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { Button, Spinner } from "@heroui/react"
import { Icon } from "@iconify/react"
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
  fetchViews,
  sheetKeys,
  updateView,
  type FilterRuleState,
  type SheetFieldSpec,
  type SheetPage,
  type SheetSort,
} from "@/app/w/(chat)/_lib/sheets"
import { SheetGrid } from "./grid"
import { ImportDialog } from "./import-dialog"
import { SheetToolbar } from "./toolbar"
import { useSheetRows } from "./use-sheet-rows"

const PERSIST_DEBOUNCE_MS = 800
const SEARCH_DEBOUNCE_MS = 300

const EMPTY_SELECTION: GridSelection = {
  columns: CompactSelection.empty(),
  rows: CompactSelection.empty(),
}

interface PersistedConfig {
  column_widths?: Record<string, number>
  filters?: FilterRuleState[]
  sort?: SheetSort | null
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
  const sort = record.sort
  if (
    sort &&
    typeof sort === "object" &&
    typeof (sort as SheetSort).field === "string"
  ) {
    config.sort = sort as SheetSort
  }
  return config
}

/**
 * Everything for one sheet page: toolbar, Glide grid, import dialog.
 * Mount with key={pageId} so all local state resets per page. Waits for the
 * page's saved view so filters/sorts/widths initialize from its config.
 */
export function SheetWorkbench({
  sheetId,
  page,
  pages,
}: {
  sheetId: string
  page: SheetPage
  pages: SheetPage[]
}) {
  const pageId = page.id ?? ""
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

  const defaultView = viewsQuery.data?.views?.[0] ?? null
  return (
    <WorkbenchInner
      sheetId={sheetId}
      page={page}
      pages={pages}
      defaultViewId={defaultView?.id ?? null}
      initialConfig={parsePersistedConfig(defaultView?.config)}
    />
  )
}

function WorkbenchInner({
  sheetId,
  page,
  pages,
  defaultViewId,
  initialConfig,
}: {
  sheetId: string
  page: SheetPage
  pages: SheetPage[]
  defaultViewId: string | null
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
  const [sort, setSort] = useState<SheetSort | null>(
    () => initialConfig.sort ?? null
  )
  const [search, setSearch] = useState("")
  const [debouncedSearch, setDebouncedSearch] = useState("")
  const [selection, setSelection] = useState<GridSelection>(EMPTY_SELECTION)
  const [importOpen, setImportOpen] = useState(false)

  useEffect(() => {
    const timer = setTimeout(
      () => setDebouncedSearch(search),
      SEARCH_DEBOUNCE_MS
    )
    return () => clearTimeout(timer)
  }, [search])

  /* ---------------- view config: debounce persist -------------------- */

  const creatingViewRef = useRef(false)
  const latestRef = useRef({ columnWidths, filterRules, sort, defaultViewId })
  useEffect(() => {
    latestRef.current = { columnWidths, filterRules, sort, defaultViewId }
  })

  const persistTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const schedulePersist = useCallback(() => {
    if (persistTimerRef.current) clearTimeout(persistTimerRef.current)
    persistTimerRef.current = setTimeout(() => {
      const latest = latestRef.current
      const config = {
        column_widths: latest.columnWidths,
        filters: latest.filterRules,
        sort: latest.sort,
      }
      if (latest.defaultViewId) {
        updateView(sheetId, pageId, latest.defaultViewId, { config }).catch(
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
  }, [pageId, queryClient, sheetId])

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
  const sorts = useMemo(() => (sort ? [sort] : undefined), [sort])

  const controller = useSheetRows({
    sheetId,
    pageId,
    filter,
    sorts,
    search: debouncedSearch || undefined,
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

  const onSortChange = useCallback(
    (nextSort: SheetSort | null) => {
      setSort(nextSort)
      schedulePersist()
    },
    [schedulePersist]
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

  /* ---------------- render ------------------------------------------ */

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <SheetToolbar
        sheetId={sheetId}
        pageId={pageId}
        pages={pages}
        fields={fields}
        search={search}
        onSearchChange={setSearch}
        filterRules={filterRules}
        onFilterRulesChange={onFilterRulesChange}
        sort={sort}
        onSortChange={onSortChange}
        selectedRowIds={selectedRowIds}
        onDeleteSelected={onDeleteSelected}
        onOpenImport={() => setImportOpen(true)}
        onAddField={onAddField}
      />

      {fields.length === 0 ? (
        <div className="flex flex-1 flex-col items-center justify-center gap-2 p-6">
          <Icon icon="lucide:columns-3" className="h-6 w-6 text-muted" />
          <p className="text-sm text-muted">
            This page has no columns yet. Add one to start entering data.
          </p>
        </div>
      ) : controller.isPending ? (
        <div className="flex flex-1 items-center justify-center">
          <Spinner size="sm" />
        </div>
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
      ) : (
        <SheetGrid
          sheetId={sheetId}
          pageId={pageId}
          pages={pages}
          fields={fields}
          controller={controller}
          columnWidths={columnWidths}
          onColumnWidthChange={onColumnWidthChange}
          selection={selection}
          onSelectionChange={setSelection}
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
