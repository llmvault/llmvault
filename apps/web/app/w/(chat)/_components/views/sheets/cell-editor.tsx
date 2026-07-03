"use client"

import { useEffect, useMemo, useRef, useState } from "react"
import {
  Button,
  Calendar,
  ComboBox,
  DateField,
  Input,
  ListBox,
  NumberField,
  Spinner,
  Tag,
  TagGroup,
  toast,
} from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { parseDate, type DateValue } from "@internationalized/date"
import { extractErrorMessage } from "@/lib/api/error"
import {
  fetchAttachmentDownloadURLs,
  queryRows,
  relationTargetPageId,
  resolvePageRef,
  rowDisplayLabel,
  selectChoices,
  sheetKeys,
  stringArrayValue,
  uploadSheetObject,
  type SheetField,
  type SheetRelationRef,
} from "@/app/w/(chat)/_lib/sheets"
import { attachmentFileName, isImageAttachmentKey } from "./cells"

export interface CellEditorTarget {
  rowId: string
  field: SheetField
  bounds: { x: number; y: number; width: number; height: number }
}

const PANEL_WIDTH = 288
const RELATION_SEARCH_DEBOUNCE_MS = 250

export function CellEditorOverlay({
  sheetId,
  pageId,
  channelId,
  target,
  value,
  relations,
  onCommit,
  onClose,
}: {
  sheetId: string
  pageId: string
  channelId: string
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
            channelId={channelId}
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
          <AppIcon icon="check" className="h-3 w-3 text-accent-foreground" />
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

/** ISO "YYYY-MM-DD" ↔ @internationalized/date. Clearing commits null. */
function DateEditor({
  value,
  onCommit,
  onClose,
}: {
  value: unknown
  onCommit: (value: unknown) => void
  onClose: () => void
}) {
  const initial = useMemo<DateValue | null>(() => {
    if (typeof value !== "string" || value.length < 10) return null
    try {
      return parseDate(value.slice(0, 10))
    } catch {
      return null
    }
  }, [value])
  const [draft, setDraft] = useState<DateValue | null>(initial)

  const commit = (next: DateValue | null, close = false) => {
    setDraft(next)
    onCommit(next ? next.toString() : null)
    if (close) onClose()
  }

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center gap-1.5">
        <DateField
          aria-label="Date"
          autoFocus
          value={draft}
          onChange={(next) => commit(next ?? null)}
          className="min-w-0 flex-1"
        >
          <DateField.Group>
            <DateField.Input>
              {(segment) => <DateField.Segment segment={segment} />}
            </DateField.Input>
          </DateField.Group>
        </DateField>
        <Button
          variant="ghost"
          size="sm"
          isIconOnly
          aria-label="Clear date"
          isDisabled={draft === null}
          onPress={() => commit(null, true)}
        >
          <AppIcon icon="x" className="h-3.5 w-3.5" />
        </Button>
      </div>
      <Calendar
        aria-label="Pick a date"
        value={draft}
        onChange={(next) => commit(next ?? null, true)}
      >
        <Calendar.Header>
          <Calendar.NavButton slot="previous">
            <AppIcon icon="chevron-left" className="h-4 w-4" />
          </Calendar.NavButton>
          <Calendar.Heading />
          <Calendar.NavButton slot="next">
            <AppIcon icon="chevron-right" className="h-4 w-4" />
          </Calendar.NavButton>
        </Calendar.Header>
        <Calendar.Grid>
          <Calendar.GridHeader>
            {(day) => <Calendar.HeaderCell>{day}</Calendar.HeaderCell>}
          </Calendar.GridHeader>
          <Calendar.GridBody>
            {(date) => <Calendar.Cell date={date} />}
          </Calendar.GridBody>
        </Calendar.Grid>
      </Calendar>
    </div>
  )
}

/** Plain NumberField; commits when the field commits (Enter / blur). */
function NumberEditor({
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
  const initial = typeof value === "number" ? value : Number.NaN

  return (
    <NumberField
      aria-label={field.name || "Number"}
      autoFocus
      defaultValue={initial}
      onChange={(next) => onCommit(Number.isNaN(next) ? null : next)}
      className="w-full"
    >
      <NumberField.Group>
        <NumberField.Input
          onKeyDown={(event) => {
            if (event.key !== "Enter") return
            // React Aria commits the typed text on Enter in the same tick;
            // defer closing so onChange fires with the committed value first.
            window.setTimeout(onClose, 0)
          }}
        />
      </NumberField.Group>
    </NumberField>
  )
}

/**
 * Relation picker. The target page may belong to a DIFFERENT sheet, so the
 * page is resolved to its owning sheet first and rows are always queried
 * through that sheet's path.
 */
function RelationEditor({
  field,
  value,
  channelId,
  relations,
  onCommit,
}: {
  field: SheetField
  value: unknown
  channelId: string
  relations: Record<string, SheetRelationRef>
  onCommit: (value: unknown) => void
}) {
  const queryClient = useQueryClient()
  const linked = stringArrayValue(value)
  const targetPageId = relationTargetPageId(field)

  const refQuery = useQuery({
    enabled: Boolean(targetPageId),
    queryKey: sheetKeys.pageRef(targetPageId ?? ""),
    queryFn: () => resolvePageRef(queryClient, channelId, targetPageId ?? ""),
    staleTime: 5 * 60_000,
  })
  const pageRef = refQuery.data ?? null

  const [input, setInput] = useState("")
  const [search, setSearch] = useState("")
  useEffect(() => {
    const timer = setTimeout(
      () => setSearch(input),
      RELATION_SEARCH_DEBOUNCE_MS
    )
    return () => clearTimeout(timer)
  }, [input])

  const optionsQuery = useQuery({
    enabled: Boolean(pageRef),
    queryKey: ["sheet-relation-options", targetPageId, search],
    queryFn: ({ signal }) =>
      queryRows(
        pageRef?.sheetId ?? "",
        targetPageId ?? "",
        { search: search || undefined, limit: 20, resolve_relations: false },
        signal
      ),
    placeholderData: (previous) => previous,
  })

  // Labels for rows linked in this editing session (they may not be in the
  // page's resolved-relations map yet).
  const [labelCache, setLabelCache] = useState<Record<string, string>>({})
  const labelFor = (id: string) =>
    labelCache[id] ?? relations[id]?.label ?? "Untitled row"

  const options = useMemo(() => {
    if (!pageRef) return []
    return (optionsQuery.data?.rows ?? []).flatMap((row) =>
      row.id ? [{ id: row.id, label: rowDisplayLabel(row, pageRef.page) }] : []
    )
  }, [optionsQuery.data, pageRef])

  if (refQuery.isPending) {
    return (
      <div className="flex justify-center py-3">
        <Spinner size="sm" />
      </div>
    )
  }

  if (!pageRef || !targetPageId) {
    return (
      <p className="px-2 py-1.5 text-sm text-muted">
        The linked page could not be found. It may have been archived.
      </p>
    )
  }

  const linkedItems = linked.map((id) => ({ id, label: labelFor(id) }))

  const toggle = (id: string, label?: string) => {
    if (label) setLabelCache((prev) => ({ ...prev, [id]: label }))
    onCommit(
      linked.includes(id)
        ? linked.filter((entry) => entry !== id)
        : [...linked, id]
    )
  }

  return (
    <div className="flex flex-col gap-1.5">
      {linkedItems.length > 0 ? (
        <TagGroup
          aria-label="Linked rows"
          size="sm"
          onRemove={(keys) => {
            const removed = new Set(Array.from(keys, String))
            onCommit(linked.filter((id) => !removed.has(id)))
          }}
          className="px-1 pt-1"
        >
          <TagGroup.List items={linkedItems}>
            {(item) => (
              <Tag id={item.id} textValue={item.label}>
                <span className="max-w-36 truncate">{item.label}</span>
                <Tag.RemoveButton />
              </Tag>
            )}
          </TagGroup.List>
        </TagGroup>
      ) : null}

      <ComboBox
        aria-label={`Search ${pageRef.page.name ?? "rows"}`}
        items={options}
        inputValue={input}
        onInputChange={setInput}
        selectedKey={null}
        onSelectionChange={(key) => {
          if (key === null) return
          const id = String(key)
          toggle(id, options.find((option) => option.id === id)?.label)
          setInput("")
        }}
        menuTrigger="focus"
        allowsEmptyCollection
      >
        <ComboBox.InputGroup>
          <Input
            autoFocus
            placeholder={`Search ${pageRef.page.name ?? "rows"}${
              pageRef.sheetName ? ` · ${pageRef.sheetName}` : ""
            }…`}
          />
          <ComboBox.Trigger />
        </ComboBox.InputGroup>
        <ComboBox.Popover className="w-72">
          <ListBox<{ id: string; label: string }>
            renderEmptyState={() => (
              <p className="px-2 py-1.5 text-sm text-muted">
                {optionsQuery.isFetching ? "Searching…" : "No matching rows."}
              </p>
            )}
          >
            {(item) => (
              <ListBox.Item id={item.id} textValue={item.label}>
                <span className="flex min-w-0 items-center gap-2">
                  {linked.includes(item.id) ? (
                    <AppIcon
                      icon="check"
                      className="h-3.5 w-3.5 shrink-0 text-accent"
                    />
                  ) : (
                    <span className="h-3.5 w-3.5 shrink-0" />
                  )}
                  <span className="truncate">{item.label}</span>
                </span>
              </ListBox.Item>
            )}
          </ListBox>
        </ComboBox.Popover>
      </ComboBox>
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

  const imageKeys = useMemo(
    () => keys.filter((key) => isImageAttachmentKey(key)),
    [keys]
  )
  const previewsQuery = useQuery({
    enabled: imageKeys.length > 0,
    queryKey: ["sheet-attachment-previews", pageId, ...imageKeys],
    queryFn: () => fetchAttachmentDownloadURLs(sheetId, pageId, imageKeys),
    staleTime: 4 * 60_000,
    refetchInterval: 4 * 60_000,
  })
  const previewUrls = previewsQuery.data?.urls ?? {}

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
        keys.map((key) => {
          const previewUrl = isImageAttachmentKey(key)
            ? previewUrls[key]
            : undefined
          return (
            <div
              key={key}
              className="flex items-center gap-1.5 rounded-lg px-2 py-1.5 hover:bg-default"
            >
              {previewUrl ? (
                // eslint-disable-next-line @next/next/no-img-element -- presigned, short-lived storage URL
                <img
                  src={previewUrl}
                  alt={attachmentFileName(key)}
                  className="h-8 w-8 shrink-0 rounded-md border border-border object-cover"
                />
              ) : (
                <AppIcon
                  icon={
                    isImageAttachmentKey(key)
                      ? "image"
                      : "paperclip"
                  }
                  className="h-3.5 w-3.5 shrink-0 text-muted"
                />
              )}
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
                  <AppIcon icon="download" className="h-3.5 w-3.5" />
                )}
              </Button>
              <Button
                variant="ghost"
                size="sm"
                isIconOnly
                aria-label="Remove attachment"
                onPress={() => onCommit(keys.filter((entry) => entry !== key))}
              >
                <AppIcon icon="trash-2" className="h-3.5 w-3.5" />
              </Button>
            </div>
          )
        })
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
          <AppIcon icon="upload" className="h-3.5 w-3.5" />
        )}
        Upload file
      </Button>
    </div>
  )
}
