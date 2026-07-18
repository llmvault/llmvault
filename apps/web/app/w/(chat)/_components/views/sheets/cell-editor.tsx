"use client"

import { useEffect } from "react"
import type { SheetField, SheetRelationRef } from "@/app/w/(chat)/_lib/sheets"
import {
  AttachmentEditor,
  DateEditor,
  MultiSelectEditor,
  NumberEditor,
  RelationEditor,
  SelectEditor,
} from "./cell-editor-fields"

export interface CellEditorTarget {
  rowId: string
  field: SheetField
  bounds: { x: number; y: number; width: number; height: number }
}

const PANEL_WIDTH = 288

export function CellEditorOverlay({
  sheetId,
  pageId,
  teamId,
  target,
  value,
  relations,
  onCommit,
  onClose,
}: {
  sheetId: string
  pageId: string
  teamId: string
  target: CellEditorTarget
  value: unknown
  relations: Record<string, SheetRelationRef>
  onCommit: (value: unknown) => void
  onClose: () => void
}) {
  const left = Math.max(
    8,
    Math.min(target.bounds.x, window.innerWidth - PANEL_WIDTH - 8)
  )
  const top = Math.min(
    target.bounds.y + target.bounds.height + 2,
    window.innerHeight - 80
  )

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose()
    }
    window.addEventListener("keydown", onKeyDown)
    return () => window.removeEventListener("keydown", onKeyDown)
  }, [onClose])

  return (
    <>
      <div className="fixed inset-0 z-40" onClick={onClose} />
      <div
        className="fixed z-50 max-h-96 overflow-y-auto rounded-xl border border-border bg-surface p-2 shadow-lg"
        style={{ left, top, width: PANEL_WIDTH }}
      >
        {target.field.type === "select" ? (
          <SelectEditor
            field={target.field}
            value={value}
            onCommit={onCommit}
            onClose={onClose}
          />
        ) : null}
        {target.field.type === "multi_select" ? (
          <MultiSelectEditor
            field={target.field}
            value={value}
            onCommit={onCommit}
          />
        ) : null}
        {target.field.type === "date" ? (
          <DateEditor value={value} onCommit={onCommit} onClose={onClose} />
        ) : null}
        {target.field.type === "number" ? (
          <NumberEditor
            field={target.field}
            value={value}
            onCommit={onCommit}
            onClose={onClose}
          />
        ) : null}
        {target.field.type === "relation" ? (
          <RelationEditor
            field={target.field}
            value={value}
            teamId={teamId}
            relations={relations}
            onCommit={onCommit}
          />
        ) : null}
        {target.field.type === "attachment" ? (
          <AttachmentEditor
            sheetId={sheetId}
            pageId={pageId}
            value={value}
            onCommit={onCommit}
          />
        ) : null}
      </div>
    </>
  )
}
