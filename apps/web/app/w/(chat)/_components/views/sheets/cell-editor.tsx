"use client"

import { useEffect, useRef, useState } from "react"
import { Button, Input, Spinner, toast } from "@heroui/react"
import { Icon } from "@iconify/react"
import { useQuery } from "@tanstack/react-query"
import { extractErrorMessage } from "@/lib/api/error"
import {
  fetchAttachmentDownloadURLs,
  queryRows,
  relationTargetPageId,
  rowDisplayLabel,
  selectChoices,
  stringArrayValue,
  uploadSheetObject,
  type SheetField,
  type SheetPage,
  type SheetRelationRef,
} from "@/app/w/(chat)/_lib/sheets"
import { attachmentFileName } from "./cells"

export interface CellEditorTarget {
  rowId: string
  field: SheetField
  bounds: { x: number; y: number; width: number; height: number }
}

const PANEL_WIDTH = 288

export function CellEditorOverlay({
  sheetId,
  pageId,
  pages,
  target,
  value,
  relations,
  onCommit,
  onClose,
}: {
  sheetId: string
  pageId: string
  pages: SheetPage[]
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
        className="fixed z-50 max-h-80 overflow-y-auto rounded-xl border border-border bg-surface p-2 shadow-lg"
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
        {target.field.type === "relation" ? (
          <RelationEditor
            sheetId={sheetId}
            field={target.field}
            pages={pages}
            value={value}
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

function ChoiceRow({
  label,
  selected,
  onPress,
}: {
  label: string
  selected: boolean
  onPress: () => void
}) {
  return (
    <button
      type="button"
      className="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm text-foreground transition-colors hover:bg-default"
      onClick={onPress}
    >
      <span
        className={`flex h-4 w-4 shrink-0 items-center justify-center rounded border ${
          selected ? "border-accent bg-accent" : "border-border"
        }`}
      >
        {selected ? (
          <Icon icon="lucide:check" className="h-3 w-3 text-accent-foreground" />
        ) : null}
      </span>
      <span className="truncate">{label}</span>
    </button>
  )
}

function SelectEditor({
  field,
  value,
  onCommit,
  onClose,
}: {
  field: SheetField
  value: unknown
  onCommit: (value: unknown) => void
  onClose: () => void
}) {
  const current = typeof value === "string" ? value : ""
  const choices = selectChoices(field)
  if (choices.length === 0) {
    return (
      <p className="px-2 py-1.5 text-sm text-muted">
        This select field has no choices yet.
      </p>
    )
  }
  return (
    <div className="flex flex-col gap-0.5">
      {choices.map((choice) => (
        <ChoiceRow
          key={choice}
          label={choice}
          selected={choice === current}
          onPress={() => {
            onCommit(choice === current ? null : choice)
            onClose()
          }}
        />
      ))}
    </div>
  )
}

function MultiSelectEditor({
  field,
  value,
  onCommit,
}: {
  field: SheetField
  value: unknown
  onCommit: (value: unknown) => void
}) {
  const selected = stringArrayValue(value)
  const choices = selectChoices(field)
  if (choices.length === 0) {
    return (
      <p className="px-2 py-1.5 text-sm text-muted">
        This select field has no choices yet.
      </p>
    )
  }
  return (
    <div className="flex flex-col gap-0.5">
      {choices.map((choice) => {
        const isSelected = selected.includes(choice)
        return (
          <ChoiceRow
            key={choice}
            label={choice}
            selected={isSelected}
            onPress={() =>
              onCommit(
                isSelected
                  ? selected.filter((entry) => entry !== choice)
                  : [...selected, choice]
              )
            }
          />
        )
      })}
    </div>
  )
}

function RelationEditor({
  sheetId,
  field,
  pages,
  value,
  relations,
  onCommit,
}: {
  sheetId: string
  field: SheetField
  pages: SheetPage[]
  value: unknown
  relations: Record<string, SheetRelationRef>
  onCommit: (value: unknown) => void
}) {
  const linked = stringArrayValue(value)
  const targetPageId = relationTargetPageId(field)
  const targetPage = pages.find((page) => page.id === targetPageId) ?? null
  const [search, setSearch] = useState("")

  const optionsQuery = useQuery({
    enabled: Boolean(targetPage?.id),
    queryKey: ["sheet-relation-options", targetPageId, search],
    queryFn: ({ signal }) =>
      queryRows(
        sheetId,
        targetPageId ?? "",
        { search: search || undefined, limit: 20, resolve_relations: false },
        signal
      ),
    placeholderData: (previous) => previous,
  })

  if (!targetPage) {
    return (
      <p className="px-2 py-1.5 text-sm text-muted">
        The linked page is not part of this sheet, so links can&apos;t be
        edited here.
      </p>
    )
  }

  const options = optionsQuery.data?.rows ?? []

  return (
    <div className="flex flex-col gap-1.5">
      {linked.length > 0 ? (
        <div className="flex flex-wrap gap-1 px-1 pt-1">
          {linked.map((id) => (
            <span
              key={id}
              className="flex items-center gap-1 rounded-full border border-border bg-default px-2 py-0.5 text-xs text-foreground"
            >
              <span className="max-w-36 truncate">
                {relations[id]?.label || "Untitled row"}
              </span>
              <button
                type="button"
                aria-label="Remove link"
                className="text-muted hover:text-foreground"
                onClick={() =>
                  onCommit(linked.filter((entry) => entry !== id))
                }
              >
                <Icon icon="lucide:x" className="h-3 w-3" />
              </button>
            </span>
          ))}
        </div>
      ) : null}
      <Input
        aria-label="Search rows"
        placeholder={`Search ${targetPage.name ?? "rows"}…`}
        value={search}
        onChange={(event) => setSearch(event.target.value)}
        autoFocus
      />
      {optionsQuery.isPending ? (
        <div className="flex justify-center py-3">
          <Spinner size="sm" />
        </div>
      ) : options.length === 0 ? (
        <p className="px-2 py-1.5 text-sm text-muted">No matching rows.</p>
      ) : (
        <div className="flex flex-col gap-0.5">
          {options.map((row) => {
            if (!row.id) return null
            const isLinked = linked.includes(row.id)
            return (
              <ChoiceRow
                key={row.id}
                label={rowDisplayLabel(row, targetPage)}
                selected={isLinked}
                onPress={() =>
                  onCommit(
                    isLinked
                      ? linked.filter((entry) => entry !== row.id)
                      : [...linked, row.id]
                  )
                }
              />
            )
          })}
        </div>
      )}
    </div>
  )
}

function AttachmentEditor({
  sheetId,
  pageId,
  value,
  onCommit,
}: {
  sheetId: string
  pageId: string
  value: unknown
  onCommit: (value: unknown) => void
}) {
  const keys = stringArrayValue(value)
  const inputRef = useRef<HTMLInputElement>(null)
  const [uploading, setUploading] = useState(false)
  const [downloadingKey, setDownloadingKey] = useState<string | null>(null)

  const upload = async (file: File) => {
    setUploading(true)
    try {
      const key = await uploadSheetObject("sheet_attachment", file)
      onCommit([...keys, key])
    } catch (error) {
      toast.danger(extractErrorMessage(error, "Could not upload the file"))
    } finally {
      setUploading(false)
    }
  }

  const download = async (key: string) => {
    setDownloadingKey(key)
    try {
      const response = await fetchAttachmentDownloadURLs(sheetId, pageId, [key])
      const url = response.urls?.[key]
      if (!url) throw new Error("No download URL was returned")
      window.open(url, "_blank", "noopener,noreferrer")
    } catch (error) {
      toast.danger(extractErrorMessage(error, "Could not download the file"))
    } finally {
      setDownloadingKey(null)
    }
  }

  return (
    <div className="flex flex-col gap-1">
      {keys.length === 0 ? (
        <p className="px-2 py-1.5 text-sm text-muted">No attachments yet.</p>
      ) : (
        keys.map((key) => (
          <div
            key={key}
            className="flex items-center gap-1.5 rounded-lg px-2 py-1.5 hover:bg-default"
          >
            <Icon
              icon="lucide:paperclip"
              className="h-3.5 w-3.5 shrink-0 text-muted"
            />
            <span className="min-w-0 flex-1 truncate text-sm text-foreground">
              {attachmentFileName(key)}
            </span>
            <Button
              variant="ghost"
              size="sm"
              isIconOnly
              aria-label="Download attachment"
              isDisabled={downloadingKey === key}
              onPress={() => void download(key)}
            >
              {downloadingKey === key ? (
                <Spinner size="sm" />
              ) : (
                <Icon icon="lucide:download" className="h-3.5 w-3.5" />
              )}
            </Button>
            <Button
              variant="ghost"
              size="sm"
              isIconOnly
              aria-label="Remove attachment"
              onPress={() => onCommit(keys.filter((entry) => entry !== key))}
            >
              <Icon icon="lucide:trash-2" className="h-3.5 w-3.5" />
            </Button>
          </div>
        ))
      )}
      <input
        ref={inputRef}
        type="file"
        className="hidden"
        onChange={(event) => {
          const file = event.target.files?.[0]
          event.target.value = ""
          if (file) void upload(file)
        }}
      />
      <Button
        variant="secondary"
        size="sm"
        isDisabled={uploading}
        onPress={() => inputRef.current?.click()}
      >
        {uploading ? (
          <Spinner size="sm" />
        ) : (
          <Icon icon="lucide:upload" className="h-3.5 w-3.5" />
        )}
        Upload file
      </Button>
    </div>
  )
}
