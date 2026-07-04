"use client"

import { memo, useEffect, useMemo, useRef, useState } from "react"
import { Button, Input, Popover, Tag, TagGroup } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import {
  filterOpIsMulti,
  filterOpLabel,
  filterOpNeedsValue,
  filterOpsForType,
  type FilterRuleState,
  type SheetField,
  type SheetSort,
} from "@/app/w/(chat)/_lib/sheets"
import { ToolbarSelect } from "./toolbar-select"

function ruleId(): string {
  return Math.random().toString(36).slice(2)
}

/** Free-text tag entry for multi-value operators (`in`). */
function TagValuesInput({
  values,
  onChange,
  ariaLabel,
}: {
  values: string[]
  onChange: (values: string[]) => void
  ariaLabel: string
}) {
  const [draft, setDraft] = useState("")

  const addDraft = () => {
    const trimmed = draft.trim()
    setDraft("")
    if (!trimmed || values.includes(trimmed)) return
    onChange([...values, trimmed])
  }

  return (
    <div className="flex min-w-0 flex-1 flex-col gap-1">
      <Input
        aria-label={ariaLabel}
        placeholder="Type a value, press Enter"
        value={draft}
        onChange={(event) => setDraft(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === "Enter") {
            event.preventDefault()
            addDraft()
          } else if (
            event.key === "Backspace" &&
            draft === "" &&
            values.length > 0
          ) {
            onChange(values.slice(0, -1))
          }
        }}
        onBlur={addDraft}
        className="h-8 text-xs"
      />
      {values.length > 0 ? (
        <TagGroup
          aria-label={`${ariaLabel} values`}
          size="sm"
          onRemove={(keys) => {
            const removed = new Set(Array.from(keys, String))
            onChange(values.filter((value) => !removed.has(value)))
          }}
        >
          <TagGroup.List items={values.map((value) => ({ id: value }))}>
            {(item) => (
              <Tag id={item.id} textValue={item.id}>
                <span className="max-w-28 truncate">{item.id}</span>
                <Tag.RemoveButton />
              </Tag>
            )}
          </TagGroup.List>
        </TagGroup>
      ) : null}
    </div>
  )
}

/**
 * Text input that debounces its committed value. Typing a filter value flows
 * all the way into the rows query key, so without this every keystroke would
 * mint a fresh cache entry / network request. Local state keeps the field
 * responsive; the parent only sees the value after the user pauses. Syncs down
 * when the external value changes (e.g. the rule's field is switched, resetting
 * the value to empty).
 */
function DebouncedValueInput({
  value,
  onChange,
  ariaLabel,
  className,
}: {
  value: string
  onChange: (value: string) => void
  ariaLabel: string
  className?: string
}) {
  const [local, setLocal] = useState(value)
  const onChangeRef = useRef(onChange)
  useEffect(() => {
    onChangeRef.current = onChange
  })

  useEffect(() => {
    setLocal(value)
  }, [value])

  useEffect(() => {
    if (local === value) return
    const timer = setTimeout(() => onChangeRef.current(local), 300)
    return () => clearTimeout(timer)
  }, [local, value])

  return (
    <Input
      aria-label={ariaLabel}
      value={local}
      onChange={(event) => setLocal(event.target.value)}
      className={className}
    />
  )
}

export const FilterPopover = memo(function FilterPopover({
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
  const fieldOptions = useMemo(
    () =>
      fields
        .filter((field) => field.id)
        .map((field) => ({ value: field.id ?? "", label: field.name ?? "" })),
    [fields]
  )

  const updateRule = (id: string, patch: Partial<FilterRuleState>) => {
    onChange(
      rules.map((rule) => (rule.id === id ? { ...rule, ...patch } : rule))
    )
  }

  return (
    <Popover>
      <Popover.Trigger className="flex h-8 items-center gap-1.5 rounded-lg border border-border px-2 text-xs text-muted transition-colors hover:bg-default">
        <AppIcon icon="list-filter" className="h-3.5 w-3.5" />
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
                <div key={rule.id} className="flex items-start gap-1.5">
                  <ToolbarSelect
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
                        values: [],
                      })
                    }}
                    options={fieldOptions}
                    className="w-28 shrink-0"
                  />
                  <ToolbarSelect
                    ariaLabel="Filter operator"
                    value={rule.op}
                    onChange={(value) => updateRule(rule.id, { op: value })}
                    options={ops.map((op) => ({
                      value: op,
                      label: filterOpLabel(op),
                    }))}
                    className="w-28 shrink-0"
                  />
                  {filterOpIsMulti(rule.op) ? (
                    <TagValuesInput
                      ariaLabel="Filter values"
                      values={rule.values ?? []}
                      onChange={(values) => updateRule(rule.id, { values })}
                    />
                  ) : filterOpNeedsValue(rule.op) ? (
                    field?.type === "checkbox" ? (
                      <ToolbarSelect
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
                      <DebouncedValueInput
                        ariaLabel="Filter value"
                        value={rule.value}
                        onChange={(value) => updateRule(rule.id, { value })}
                        className="h-8 min-w-0 flex-1 text-xs"
                      />
                    )
                  ) : (
                    <div className="flex-1" />
                  )}
                  <button
                    type="button"
                    aria-label="Remove filter"
                    className="mt-1.5 rounded p-1 text-muted transition-colors hover:bg-default hover:text-foreground"
                    onClick={() =>
                      onChange(rules.filter((entry) => entry.id !== rule.id))
                    }
                  >
                    <AppIcon icon="x" className="h-3.5 w-3.5" />
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
              <AppIcon icon="plus" className="h-3.5 w-3.5" />
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
})

export const SortPopover = memo(function SortPopover({
  fields,
  sorts,
  onChange,
}: {
  fields: SheetField[]
  sorts: SheetSort[]
  onChange: (sorts: SheetSort[]) => void
}) {
  const fieldOptions = useMemo(
    () =>
      fields
        .filter((field) => field.id)
        .map((field) => ({ value: field.id ?? "", label: field.name ?? "" })),
    [fields]
  )

  const usedFields = new Set(sorts.map((sort) => sort.field))
  const firstUnused = fieldOptions.find(
    (option) => !usedFields.has(option.value)
  )

  const updateSort = (index: number, patch: Partial<SheetSort>) => {
    onChange(
      sorts.map((sort, i) => (i === index ? { ...sort, ...patch } : sort))
    )
  }

  const moveSort = (index: number, direction: -1 | 1) => {
    const target = index + direction
    if (target < 0 || target >= sorts.length) return
    const next = [...sorts]
    const [entry] = next.splice(index, 1)
    next.splice(target, 0, entry!)
    onChange(next)
  }

  return (
    <Popover>
      <Popover.Trigger className="flex h-8 items-center gap-1.5 rounded-lg border border-border px-2 text-xs text-muted transition-colors hover:bg-default">
        <AppIcon icon="arrow-up-down" className="h-3.5 w-3.5" />
        Sort
        {sorts.length > 0 ? (
          <span className="rounded-full bg-accent px-1.5 text-[10px] text-accent-foreground">
            {sorts.length}
          </span>
        ) : null}
      </Popover.Trigger>
      <Popover.Content className="w-96 rounded-2xl border border-border p-3">
        <Popover.Dialog className="flex w-full flex-col gap-2 p-0">
          {sorts.length === 0 ? (
            <p className="text-xs text-muted">
              Manual order. Add a sort to order rows by a column.
            </p>
          ) : (
            sorts.map((sort, index) => (
              <div
                key={sort.field ?? index}
                className="flex items-center gap-1.5"
              >
                <ToolbarSelect
                  ariaLabel={`Sort ${index + 1} field`}
                  value={sort.field ?? ""}
                  onChange={(value) => updateSort(index, { field: value })}
                  options={fieldOptions.filter(
                    (option) =>
                      option.value === sort.field ||
                      !usedFields.has(option.value)
                  )}
                  className="flex-1"
                />
                <Button
                  variant="ghost"
                  size="sm"
                  onPress={() => updateSort(index, { desc: !sort.desc })}
                >
                  <AppIcon
                    icon={
                      sort.desc ? "arrow-down-a-z" : "arrow-up-a-z"
                    }
                    className="h-3.5 w-3.5"
                  />
                  {sort.desc ? "Desc" : "Asc"}
                </Button>
                <button
                  type="button"
                  aria-label="Move sort up"
                  disabled={index === 0}
                  className="rounded p-1 text-muted transition-colors hover:bg-default hover:text-foreground disabled:opacity-40"
                  onClick={() => moveSort(index, -1)}
                >
                  <AppIcon icon="chevron-up" className="h-3.5 w-3.5" />
                </button>
                <button
                  type="button"
                  aria-label="Move sort down"
                  disabled={index === sorts.length - 1}
                  className="rounded p-1 text-muted transition-colors hover:bg-default hover:text-foreground disabled:opacity-40"
                  onClick={() => moveSort(index, 1)}
                >
                  <AppIcon icon="chevron-down" className="h-3.5 w-3.5" />
                </button>
                <button
                  type="button"
                  aria-label="Remove sort"
                  className="rounded p-1 text-muted transition-colors hover:bg-default hover:text-foreground"
                  onClick={() =>
                    onChange(sorts.filter((_, i) => i !== index))
                  }
                >
                  <AppIcon icon="x" className="h-3.5 w-3.5" />
                </button>
              </div>
            ))
          )}
          <div className="flex items-center justify-between">
            <Button
              variant="ghost"
              size="sm"
              isDisabled={!firstUnused}
              onPress={() => {
                if (!firstUnused) return
                onChange([...sorts, { field: firstUnused.value }])
              }}
            >
              <AppIcon icon="plus" className="h-3.5 w-3.5" />
              Add sort
            </Button>
            {sorts.length > 0 ? (
              <Button variant="ghost" size="sm" onPress={() => onChange([])}>
                Clear all
              </Button>
            ) : null}
          </div>
        </Popover.Dialog>
      </Popover.Content>
    </Popover>
  )
})
