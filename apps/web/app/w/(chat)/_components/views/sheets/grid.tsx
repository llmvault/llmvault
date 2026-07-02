"use client"

import "@glideapps/glide-data-grid/dist/index.css"

import { useCallback, useMemo, useRef, useState } from "react"
import {
  DataEditor,
  GridCellKind,
  type DataEditorRef,
  type EditableGridCell,
  type GridCell,
  type GridColumn,
  type GridSelection,
  type Item,
  type Rectangle,
} from "@glideapps/glide-data-grid"
import type { SheetField, SheetPage } from "@/app/w/(chat)/_lib/sheets"
import { CellEditorOverlay, type CellEditorTarget } from "./cell-editor"
import { cellForValue, fieldHeaderIcon, usesCustomEditor, valueFromEditedCell } from "./cells"
import { useGlideTheme } from "./use-glide-theme"
import type { SheetRowsController } from "./use-sheet-rows"

const LOADING_CELL: GridCell = {
  kind: GridCellKind.Loading,
  allowOverlay: false,
}

function defaultWidthForType(type: string | undefined): number {
  switch (type) {
    case "long_text":
      return 280
    case "number":
      return 110
    case "checkbox":
      return 90
    case "date":
      return 130
    case "select":
      return 150
    case "phone":
      return 150
    case "url":
    case "email":
    case "multi_select":
    case "attachment":
    case "relation":
      return 200
    default:
      return 180
  }
}

export function SheetGrid({
  sheetId,
  pageId,
  pages,
  fields,
  controller,
  columnWidths,
  onColumnWidthChange,
  selection,
  onSelectionChange,
}: {
  sheetId: string
  pageId: string
  pages: SheetPage[]
  fields: SheetField[]
  controller: SheetRowsController
  columnWidths: Record<string, number>
  onColumnWidthChange: (fieldId: string, width: number) => void
  selection: GridSelection
  onSelectionChange: (selection: GridSelection) => void
}) {
  const theme = useGlideTheme()
  const gridRef = useRef<DataEditorRef | null>(null)
  const [editor, setEditor] = useState<CellEditorTarget | null>(null)

  const columns = useMemo<GridColumn[]>(
    () =>
      fields.map((field) => ({
        id: field.id,
        title: field.name ?? "",
        icon: fieldHeaderIcon(field.type),
        width:
          columnWidths[field.id ?? ""] ?? defaultWidthForType(field.type),
      })),
    [fields, columnWidths]
  )

  const getCellContent = useCallback(
    ([col, row]: Item): GridCell => {
      const field = fields[col]
      const sheetRow = controller.rows[row]
      if (!field?.id || !sheetRow) return LOADING_CELL
      return cellForValue(
        field,
        sheetRow.data?.[field.id],
        controller.relations
      )
    },
    [fields, controller.rows, controller.relations]
  )

  const onCellEdited = useCallback(
    ([col, row]: Item, newValue: EditableGridCell) => {
      const field = fields[col]
      const sheetRow = controller.rows[row]
      if (!field?.id || !sheetRow?.id) return
      if (usesCustomEditor(field.type)) return
      const parsed = valueFromEditedCell(field, newValue)
      if (!parsed) return
      controller.updateCell(sheetRow.id, field.id, parsed.value)
    },
    [fields, controller]
  )

  const onCellActivated = useCallback(
    (cell: Item) => {
      const [col, row] = cell
      const field = fields[col]
      const sheetRow = controller.rows[row]
      if (!field?.id || !sheetRow?.id || !usesCustomEditor(field.type)) return
      const bounds = gridRef.current?.getBounds(col, row)
      if (!bounds) return
      setEditor({ rowId: sheetRow.id, field, bounds })
    },
    [fields, controller.rows]
  )

  const onVisibleRegionChanged = useCallback(
    (range: Rectangle) => {
      controller.onVisibleRowsChanged(range.y + range.height)
    },
    [controller]
  )

  const onColumnResize = useCallback(
    (column: GridColumn, newSize: number) => {
      if (column.id) onColumnWidthChange(column.id, newSize)
    },
    [onColumnWidthChange]
  )

  const onRowAppended = useCallback(
    () => controller.appendRow(),
    [controller]
  )

  const editingValue = useMemo(() => {
    if (!editor?.field.id) return undefined
    const row = controller.rows.find((entry) => entry.id === editor.rowId)
    return row?.data?.[editor.field.id]
  }, [controller.rows, editor])

  return (
    <div className="relative min-h-0 flex-1">
      <DataEditor
        ref={gridRef}
        columns={columns}
        rows={controller.gridRowCount}
        getCellContent={getCellContent}
        onCellEdited={onCellEdited}
        onCellActivated={onCellActivated}
        onVisibleRegionChanged={onVisibleRegionChanged}
        onColumnResize={onColumnResize}
        onRowAppended={onRowAppended}
        trailingRowOptions={{ tint: true, hint: "New row" }}
        gridSelection={selection}
        onGridSelectionChange={onSelectionChange}
        rowMarkers="both"
        rowSelect="multi"
        theme={theme}
        smoothScrollX
        smoothScrollY
        width="100%"
        height="100%"
      />
      {editor ? (
        <CellEditorOverlay
          sheetId={sheetId}
          pageId={pageId}
          pages={pages}
          target={editor}
          value={editingValue}
          relations={controller.relations}
          onCommit={(value) => {
            if (editor.field.id) {
              controller.updateCell(editor.rowId, editor.field.id, value)
            }
          }}
          onClose={() => setEditor(null)}
        />
      ) : null}
      {/* Glide renders its built-in overlay editors into this portal. */}
      <div
        id="portal"
        style={{ position: "fixed", left: 0, top: 0, zIndex: 60 }}
      />
    </div>
  )
}
