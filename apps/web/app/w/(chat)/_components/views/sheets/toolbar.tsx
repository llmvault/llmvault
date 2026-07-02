"use client"

import { useState } from "react"
import { Button, Input, Popover, toast } from "@heroui/react"
import { Icon } from "@iconify/react"
import { extractErrorMessage } from "@/lib/api/error"
import {
  exportCsvUrl,
  filterOpLabel,
  filterOpNeedsValue,
  filterOpsForType,
  SHEET_FIELD_TYPES,
  type FilterRuleState,
  type SheetField,
  type SheetFieldSpec,
  type SheetPage,
  type SheetSort,
} from "@/app/w/(chat)/_lib/sheets"
import { fieldTypeIcon } from "./cells"
import { UndoPopover } from "./undo-popover"

function ruleId(): string {
  return Math.random().toString(36).slice(2)
}

/** Compact native select styled with design tokens (dense toolbar rows). */
function TokenSelect({
  value,
  onChange,
  options,
  ariaLabel,
  className = "",
}: {
  value: string
  onChange: (value: string) => void
  options: { value: string; label: string }[]
  ariaLabel: string
  className?: string
}) {
  return (
    <select
      aria-label={ariaLabel}
      value={value}
      onChange={(event) => onChange(event.target.value)}
      className={`h-8 min-w-0 rounded-lg border border-border bg-surface px-2 text-xs text-foreground outline-none focus:border-accent ${className}`}
    >
      {options.map((option) => (
        <option key={option.value} value={option.value}>
          {option.label}
        </option>
      ))}
    </select>
  )
}

export function SheetToolbar({
  sheetId,
  pageId,
  pages,
  fields,
  search,
  onSearchChange,
  filterRules,
  onFilterRulesChange,
  sort,
  onSortChange,
  selectedRowIds,
  onDeleteSelected,
  onOpenImport,
  onAddField,
}: {
  sheetId: string
  pageId: string
  pages: SheetPage[]
  fields: SheetField[]
  search: string
  onSearchChange: (value: string) => void
  filterRules: FilterRuleState[]
  onFilterRulesChange: (rules: FilterRuleState[]) => void
  sort: SheetSort | null
  onSortChange: (sort: SheetSort | null) => void
  selectedRowIds: string[]
  onDeleteSelected: () => void
  onOpenImport: () => void
  onAddField: (spec: SheetFieldSpec) => Promise<void>
}) {
  const activeFilterCount = filterRules.length

  return (
    <div className="flex shrink-0 flex-wrap items-center gap-1.5 border-b border-border px-2 py-1.5">
      <div className="relative">
        <Icon
          icon="lucide:search"
          className="pointer-events-none absolute top-1/2 left-2 h-3.5 w-3.5 -translate-y-1/2 text-muted"
        />
        <input
          aria-label="Search rows"
          placeholder="Search…"
          value={search}
          onChange={(event) => onSearchChange(event.target.value)}
          className="h-8 w-40 rounded-lg border border-border bg-surface pr-2 pl-7 text-xs text-foreground outline-none placeholder:text-muted focus:border-accent"
        />
      </div>

      <FilterPopover
        fields={fields}
        rules={filterRules}
        onChange={onFilterRulesChange}
        activeCount={activeFilterCount}
      />
      <SortPopover fields={fields} sort={sort} onChange={onSortChange} />

      <div className="min-w-2 flex-1" />

      {selectedRowIds.length > 0 ? (
        <Button variant="ghost" size="sm" onPress={onDeleteSelected}>
          <Icon icon="lucide:trash-2" className="h-3.5 w-3.5" />
          Delete {selectedRowIds.length}
        </Button>
      ) : null}

      <AddFieldPopover pages={pages} onAddField={onAddField} />

      <Button variant="ghost" size="sm" onPress={onOpenImport}>
        <Icon icon="lucide:file-up" className="h-3.5 w-3.5" />
        Import
      </Button>

      <Button
        variant="ghost"
        size="sm"
        onPress={() => {
          window.open(exportCsvUrl(sheetId, pageId), "_blank")
        }}
      >
        <Icon icon="lucide:file-down" className="h-3.5 w-3.5" />
        Export
      </Button>

      <UndoPopover sheetId={sheetId} pageId={pageId} />
    </div>
  )
}

function FilterPopover({
  fields,
  rules,
  onChange,
  activeCount,
}: {
  fields: SheetField[]
  rules: FilterRuleState[]
  onChange: (rules: FilterRuleState[]) => void
  activeCount: number
}) {
  const fieldOptions = fields
    .filter((field) => field.id)
    .map((field) => ({ value: field.id ?? "", label: field.name ?? "" }))

  const updateRule = (id: string, patch: Partial<FilterRuleState>) => {
    onChange(
      rules.map((rule) => (rule.id === id ? { ...rule, ...patch } : rule))
    )
  }

  return (
    <Popover>
      <Popover.Trigger className="flex h-8 items-center gap-1.5 rounded-lg border border-border px-2 text-xs text-muted transition-colors hover:bg-default">
        <Icon icon="lucide:list-filter" className="h-3.5 w-3.5" />
        Filter
        {activeCount > 0 ? (
          <span className="rounded-full bg-accent px-1.5 text-[10px] text-accent-foreground">
            {activeCount}
          </span>
        ) : null}
      </Popover.Trigger>
      <Popover.Content className="w-96 rounded-2xl border border-border p-3">
        <Popover.Dialog className="flex w-full flex-col gap-2 p-0">
          {rules.length === 0 ? (
            <p className="text-xs text-muted">No filters applied.</p>
          ) : (
            rules.map((rule) => {
              const field = fields.find((entry) => entry.id === rule.field)
              const ops = filterOpsForType(field?.type)
              return (
                <div key={rule.id} className="flex items-center gap-1.5">
                  <TokenSelect
                    ariaLabel="Filter field"
                    value={rule.field}
                    onChange={(value) => {
                      const nextField = fields.find(
                        (entry) => entry.id === value
                      )
                      const nextOps = filterOpsForType(nextField?.type)
                      updateRule(rule.id, {
                        field: value,
                        op: nextOps.includes(rule.op) ? rule.op : nextOps[0]!,
                        value: "",
                      })
                    }}
                    options={fieldOptions}
                    className="w-28"
                  />
                  <TokenSelect
                    ariaLabel="Filter operator"
                    value={rule.op}
                    onChange={(value) => updateRule(rule.id, { op: value })}
                    options={ops.map((op) => ({
                      value: op,
                      label: filterOpLabel(op),
                    }))}
                    className="w-28"
                  />
                  {filterOpNeedsValue(rule.op) ? (
                    field?.type === "checkbox" ? (
                      <TokenSelect
                        ariaLabel="Filter value"
                        value={rule.value || "true"}
                        onChange={(value) => updateRule(rule.id, { value })}
                        options={[
                          { value: "true", label: "checked" },
                          { value: "false", label: "unchecked" },
                        ]}
                        className="flex-1"
                      />
                    ) : (
                      <input
                        aria-label="Filter value"
                        value={rule.value}
                        onChange={(event) =>
                          updateRule(rule.id, { value: event.target.value })
                        }
                        className="h-8 min-w-0 flex-1 rounded-lg border border-border bg-surface px-2 text-xs text-foreground outline-none focus:border-accent"
                      />
                    )
                  ) : (
                    <div className="flex-1" />
                  )}
                  <button
                    type="button"
                    aria-label="Remove filter"
                    className="rounded p-1 text-muted transition-colors hover:bg-default hover:text-foreground"
                    onClick={() =>
                      onChange(rules.filter((entry) => entry.id !== rule.id))
                    }
                  >
                    <Icon icon="lucide:x" className="h-3.5 w-3.5" />
                  </button>
                </div>
              )
            })
          )}
          <div className="flex items-center justify-between">
            <Button
              variant="ghost"
              size="sm"
              isDisabled={fieldOptions.length === 0}
              onPress={() => {
                const firstField = fields.find((entry) => entry.id)
                if (!firstField?.id) return
                onChange([
                  ...rules,
                  {
                    id: ruleId(),
                    field: firstField.id,
                    op: filterOpsForType(firstField.type)[0]!,
                    value: "",
                  },
                ])
              }}
            >
              <Icon icon="lucide:plus" className="h-3.5 w-3.5" />
              Add filter
            </Button>
            {rules.length > 0 ? (
              <Button variant="ghost" size="sm" onPress={() => onChange([])}>
                Clear all
              </Button>
            ) : null}
          </div>
        </Popover.Dialog>
      </Popover.Content>
    </Popover>
  )
}

function SortPopover({
  fields,
  sort,
  onChange,
}: {
  fields: SheetField[]
  sort: SheetSort | null
  onChange: (sort: SheetSort | null) => void
}) {
  const fieldOptions = [
    { value: "", label: "Manual order" },
    ...fields
      .filter((field) => field.id)
      .map((field) => ({ value: field.id ?? "", label: field.name ?? "" })),
  ]

  return (
    <Popover>
      <Popover.Trigger className="flex h-8 items-center gap-1.5 rounded-lg border border-border px-2 text-xs text-muted transition-colors hover:bg-default">
        <Icon icon="lucide:arrow-up-down" className="h-3.5 w-3.5" />
        Sort
        {sort ? (
          <span className="rounded-full bg-accent px-1.5 text-[10px] text-accent-foreground">
            1
          </span>
        ) : null}
      </Popover.Trigger>
      <Popover.Content className="w-72 rounded-2xl border border-border p-3">
        <Popover.Dialog className="flex w-full flex-col gap-2 p-0">
          <div className="flex items-center gap-1.5">
            <TokenSelect
              ariaLabel="Sort field"
              value={sort?.field ?? ""}
              onChange={(value) =>
                onChange(value ? { field: value, desc: sort?.desc } : null)
              }
              options={fieldOptions}
              className="flex-1"
            />
            <Button
              variant="ghost"
              size="sm"
              isDisabled={!sort}
              onPress={() =>
                sort && onChange({ field: sort.field, desc: !sort.desc })
              }
            >
              <Icon
                icon={
                  sort?.desc ? "lucide:arrow-down-a-z" : "lucide:arrow-up-a-z"
                }
                className="h-3.5 w-3.5"
              />
              {sort?.desc ? "Desc" : "Asc"}
            </Button>
          </div>
        </Popover.Dialog>
      </Popover.Content>
    </Popover>
  )
}

function AddFieldPopover({
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
      <Popover.Trigger className="flex h-8 items-center gap-1.5 rounded-lg border border-border px-2 text-xs text-muted transition-colors hover:bg-default">
        <Icon icon="lucide:columns-3" className="h-3.5 w-3.5" />
        Add column
      </Popover.Trigger>
      <Popover.Content className="w-72 rounded-2xl border border-border p-3">
        <Popover.Dialog className="flex w-full flex-col gap-2 p-0">
          <Input
            aria-label="Column name"
            placeholder="Column name"
            value={name}
            onChange={(event) => setName(event.target.value)}
            autoFocus
          />
          <div className="flex items-center gap-1.5">
            <Icon
              icon={fieldTypeIcon(type)}
              className="h-3.5 w-3.5 shrink-0 text-muted"
            />
            <TokenSelect
              ariaLabel="Column type"
              value={type}
              onChange={setType}
              options={SHEET_FIELD_TYPES.map((entry) => ({
                value: entry,
                label: entry.replace("_", " "),
              }))}
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
            <TokenSelect
              ariaLabel="Linked page"
              value={targetPageId}
              onChange={setTargetPageId}
              options={[
                { value: "", label: "Select linked page…" },
                ...pages
                  .filter((page) => page.id)
                  .map((page) => ({
                    value: page.id ?? "",
                    label: page.name ?? "",
                  })),
              ]}
              className="w-full"
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
}
