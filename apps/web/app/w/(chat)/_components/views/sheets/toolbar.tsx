"use client"

import { memo, useEffect, useRef, useState } from "react"
import { Button, SearchField, Toolbar } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import {
  exportCsvUrl,
  type FilterRuleState,
  type SheetField,
  type SheetFieldSpec,
  type SheetPage,
  type SheetSort,
  type SheetView,
} from "@/app/w/(chat)/_lib/sheets"
import { AddFieldPopover, HiddenColumnsPopover } from "./toolbar-fields"
import { FilterPopover, SortPopover } from "./toolbar-filters"
import { ViewSwitcher } from "./toolbar-views"
import { UndoPopover } from "./undo-popover"

const SEARCH_DEBOUNCE_MS = 300

/**
 * Search input that owns its own text state and debounces the committed value
 * up to the workbench. Isolating it here means every keystroke re-renders only
 * this leaf — not the whole toolbar (and its always-mounted Selects). `onSearch`
 * must be referentially stable; `resetSignal` clears the field imperatively
 * (e.g. the "Clear filters" empty state).
 */
const SearchBox = memo(function SearchBox({
  onSearch,
  resetSignal,
}: {
  onSearch: (value: string) => void
  resetSignal: number
}) {
  const [value, setValue] = useState("")
  const onSearchRef = useRef(onSearch)
  useEffect(() => {
    onSearchRef.current = onSearch
  })

  useEffect(() => {
    const timer = setTimeout(() => onSearchRef.current(value), SEARCH_DEBOUNCE_MS)
    return () => clearTimeout(timer)
  }, [value])

  const firstReset = useRef(true)
  useEffect(() => {
    if (firstReset.current) {
      firstReset.current = false
      return
    }
    setValue("")
    onSearchRef.current("")
  }, [resetSignal])

  return (
    <SearchField
      aria-label="Search rows"
      value={value}
      onChange={setValue}
      className="w-44"
    >
      <SearchField.Group className="h-8 border border-border bg-surface">
        <SearchField.SearchIcon className="h-3.5 w-3.5 text-muted" />
        <SearchField.Input placeholder="Search…" className="text-xs" />
        <SearchField.ClearButton />
      </SearchField.Group>
    </SearchField>
  )
})

export const SheetToolbar = memo(function SheetToolbar({
  sheetId,
  pageId,
  pages,
  fields,
  onSearchChange,
  searchResetSignal,
  filterRules,
  onFilterRulesChange,
  sorts,
  onSortsChange,
  views,
  activeViewId,
  onSelectView,
  onSaveAsView,
  onRenameView,
  onDeleteView,
  selectedRowIds,
  onDeleteSelected,
  onOpenImport,
  onAddField,
  hiddenFields,
  onUnhideField,
}: {
  sheetId: string
  pageId: string
  pages: SheetPage[]
  fields: SheetField[]
  onSearchChange: (value: string) => void
  searchResetSignal: number
  filterRules: FilterRuleState[]
  onFilterRulesChange: (rules: FilterRuleState[]) => void
  sorts: SheetSort[]
  onSortsChange: (sorts: SheetSort[]) => void
  views: SheetView[]
  activeViewId: string | null
  onSelectView: (viewId: string | null) => void
  onSaveAsView: (name: string) => Promise<void>
  onRenameView: (name: string) => Promise<void>
  onDeleteView: () => Promise<void>
  selectedRowIds: string[]
  onDeleteSelected: () => void
  onOpenImport: () => void
  onAddField: (spec: SheetFieldSpec) => Promise<void>
  hiddenFields: SheetField[]
  onUnhideField: (fieldId: string) => void
}) {
  const activeFilterCount = filterRules.length

  return (
    <Toolbar
      aria-label="Sheet tools"
      className="flex shrink-0 flex-wrap items-center gap-1.5 border-b border-border px-2 py-1.5"
    >
      <SearchBox onSearch={onSearchChange} resetSignal={searchResetSignal} />

      <FilterPopover
        fields={fields}
        rules={filterRules}
        onChange={onFilterRulesChange}
        activeCount={activeFilterCount}
      />
      <SortPopover fields={fields} sorts={sorts} onChange={onSortsChange} />
      {hiddenFields.length > 0 ? (
        <HiddenColumnsPopover
          hiddenFields={hiddenFields}
          onUnhideField={onUnhideField}
        />
      ) : null}
      <ViewSwitcher
        views={views}
        activeViewId={activeViewId}
        onSelectView={onSelectView}
        onSaveAsView={onSaveAsView}
        onRenameView={onRenameView}
        onDeleteView={onDeleteView}
      />

      <div className="min-w-2 flex-1" />

      {selectedRowIds.length > 0 ? (
        <Button variant="ghost" size="sm" onPress={onDeleteSelected}>
          <AppIcon icon="trash-2" className="h-3.5 w-3.5" />
          Delete {selectedRowIds.length}
        </Button>
      ) : null}

      <AddFieldPopover pages={pages} onAddField={onAddField} />

      <Button variant="ghost" size="sm" onPress={onOpenImport}>
        <AppIcon icon="file-up" className="h-3.5 w-3.5" />
        Import
      </Button>

      <Button
        variant="ghost"
        size="sm"
        onPress={() => {
          window.open(exportCsvUrl(sheetId, pageId), "_blank")
        }}
      >
        <AppIcon icon="file-down" className="h-3.5 w-3.5" />
        Export
      </Button>

      <UndoPopover sheetId={sheetId} pageId={pageId} />
    </Toolbar>
  )
})
