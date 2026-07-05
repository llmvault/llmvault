"use client"

import { memo, useMemo, useState } from "react"
import { Button, Input, Popover, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import {
  SHEET_FIELD_TYPES,
  type SheetField,
  type SheetFieldSpec,
  type SheetPage,
} from "@/app/w/(chat)/_lib/sheets"
import { fieldTypeIcon } from "./cells"
import { ToolbarSelect } from "./toolbar-select"

/** Static — the set of field types never changes at runtime. */
const FIELD_TYPE_OPTIONS = SHEET_FIELD_TYPES.map((entry) => ({
  value: entry,
  label: entry.replace("_", " "),
}))

export const HiddenColumnsPopover = memo(function HiddenColumnsPopover({
  hiddenFields,
  onUnhideField,
}: {
  hiddenFields: SheetField[]
  onUnhideField: (fieldId: string) => void
}) {
  return (
    <Popover>
      <Popover.Trigger className="flex h-8 items-center gap-1.5 border border-border px-2 text-xs text-muted transition-colors hover:bg-default">
        <AppIcon icon="eye-off" className="h-3.5 w-3.5" />
        Hidden
        <span className="rounded-full bg-accent px-1.5 text-[10px] text-accent-foreground">
          {hiddenFields.length}
        </span>
      </Popover.Trigger>
      <Popover.Content className="w-64 border border-border p-3">
        <Popover.Dialog className="flex w-full flex-col gap-0.5 p-0">
          {hiddenFields.map((field) => (
            <button
              key={field.id}
              type="button"
              className="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm text-foreground transition-colors hover:bg-default"
              onClick={() => field.id && onUnhideField(field.id)}
            >
              <AppIcon
                icon={fieldTypeIcon(field.type)}
                className="h-3.5 w-3.5 shrink-0 text-muted"
              />
              <span className="min-w-0 flex-1 truncate">{field.name}</span>
              <AppIcon icon="eye" className="h-3.5 w-3.5 text-muted" />
            </button>
          ))}
        </Popover.Dialog>
      </Popover.Content>
    </Popover>
  )
})

export const AddFieldPopover = memo(function AddFieldPopover({
  pages,
  onAddField,
}: {
  pages: SheetPage[]
  onAddField: (spec: SheetFieldSpec) => Promise<void>
}) {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState("")
  const [type, setType] = useState("text")
  const [choices, setChoices] = useState("")
  const [targetPageId, setTargetPageId] = useState("")
  const [saving, setSaving] = useState(false)

  const pageOptions = useMemo(
    () =>
      pages
        .filter((page) => page.id)
        .map((page) => ({ value: page.id ?? "", label: page.name ?? "" })),
    [pages]
  )

  const isSelect = type === "select" || type === "multi_select"
  const isRelation = type === "relation"
  const canSave =
    name.trim().length > 0 &&
    (!isSelect || choices.trim().length > 0) &&
    (!isRelation || targetPageId.length > 0)

  const submit = async () => {
    if (!canSave || saving) return
    const options: Record<string, unknown> = {}
    if (isSelect) {
      options.choices = choices
        .split(",")
        .map((choice) => choice.trim())
        .filter(Boolean)
    }
    if (isRelation) options.target_page_id = targetPageId
    setSaving(true)
    try {
      await onAddField({
        name: name.trim(),
        type,
        ...(Object.keys(options).length > 0 ? { options } : {}),
      })
      setName("")
      setChoices("")
      setTargetPageId("")
      setType("text")
      setOpen(false)
    } catch (error) {
      toast.danger(extractErrorMessage(error, "Could not add the column"))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Popover isOpen={open} onOpenChange={setOpen}>
      <Popover.Trigger className="flex h-8 items-center gap-1.5 border border-border px-2 text-xs text-muted transition-colors hover:bg-default">
        <AppIcon icon="columns-3" className="h-3.5 w-3.5" />
        Add column
      </Popover.Trigger>
      <Popover.Content className="w-72 border border-border p-3">
        <Popover.Dialog className="flex w-full flex-col gap-2 p-0">
          <Input
            aria-label="Column name"
            placeholder="Column name"
            value={name}
            onChange={(event) => setName(event.target.value)}
            autoFocus
          />
          <div className="flex items-center gap-1.5">
            <AppIcon
              icon={fieldTypeIcon(type)}
              className="h-3.5 w-3.5 shrink-0 text-muted"
            />
            <ToolbarSelect
              ariaLabel="Column type"
              value={type}
              onChange={setType}
              options={FIELD_TYPE_OPTIONS}
              className="flex-1"
            />
          </div>
          {isSelect ? (
            <Input
              aria-label="Choices"
              placeholder="Choices (comma separated)"
              value={choices}
              onChange={(event) => setChoices(event.target.value)}
            />
          ) : null}
          {isRelation ? (
            <ToolbarSelect
              ariaLabel="Linked page"
              value={targetPageId}
              onChange={setTargetPageId}
              options={pageOptions}
              className="w-full"
              placeholder="Select linked page…"
            />
          ) : null}
          <Button
            size="sm"
            isDisabled={!canSave || saving}
            onPress={() => void submit()}
          >
            Add column
          </Button>
        </Popover.Dialog>
      </Popover.Content>
    </Popover>
  )
})
