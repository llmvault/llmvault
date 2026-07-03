"use client"

import { useState } from "react"
import {
  AlertDialog,
  Button,
  Dropdown,
  FieldError,
  Input,
  Label,
  ListBox,
  Modal,
  Popover,
  SearchField,
  Select,
  Spinner,
  Tag,
  TagGroup,
  TextField,
  Toolbar,
  toast,
} from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import {
  exportCsvUrl,
  filterOpIsMulti,
  filterOpLabel,
  filterOpNeedsValue,
  filterOpsForType,
  SHEET_FIELD_TYPES,
  type FilterRuleState,
  type SheetField,
  type SheetFieldSpec,
  type SheetPage,
  type SheetSort,
  type SheetView,
} from "@/app/w/(chat)/_lib/sheets"
import { fieldTypeIcon } from "./cells"
import { UndoPopover } from "./undo-popover"

function ruleId(): string {
  return Math.random().toString(36).slice(2)
}

/** Compact HeroUI Select + ListBox for dense toolbar rows. */
function ToolbarSelect({
  value,
  onChange,
  options,
  ariaLabel,
  className = "",
  placeholder = "Select…",
}: {
  value: string
  onChange: (value: string) => void
  options: { value: string; label: string }[]
  ariaLabel: string
  className?: string
  placeholder?: string
}) {
  const selectedLabel = options.find((option) => option.value === value)?.label
  return (
    <Select
      aria-label={ariaLabel}
      selectedKey={value || null}
      onSelectionChange={(key) => {
        if (key !== null) onChange(String(key))
      }}
      className={`min-w-0 ${className}`}
    >
      <Select.Trigger className="flex h-8 w-full min-w-0 items-center justify-between gap-1 rounded-lg border border-border bg-surface px-2 text-xs text-foreground">
        <span className={`truncate ${selectedLabel ? "" : "text-muted"}`}>
          {selectedLabel ?? placeholder}
        </span>
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover className="min-w-44 rounded-xl p-1">
        <ListBox>
          {options.map((option) => (
            <ListBox.Item
              key={option.value}
              id={option.value}
              textValue={option.label}
            >
              <span className="text-xs">{option.label}</span>
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
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
  search: string
  onSearchChange: (value: string) => void
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
      <SearchField
        aria-label="Search rows"
        value={search}
        onChange={onSearchChange}
        className="w-44"
      >
        <SearchField.Group className="h-8 rounded-lg border border-border bg-surface">
          <SearchField.SearchIcon className="h-3.5 w-3.5 text-muted" />
          <SearchField.Input placeholder="Search…" className="text-xs" />
          <SearchField.ClearButton />
        </SearchField.Group>
      </SearchField>

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
}

/* ------------------------------------------------------------------ */
/* Views                                                               */
/* ------------------------------------------------------------------ */

function ViewSwitcher({
  views,
  activeViewId,
  onSelectView,
  onSaveAsView,
  onRenameView,
  onDeleteView,
}: {
  views: SheetView[]
  activeViewId: string | null
  onSelectView: (viewId: string | null) => void
  onSaveAsView: (name: string) => Promise<void>
  onRenameView: (name: string) => Promise<void>
  onDeleteView: () => Promise<void>
}) {
  const [dialog, setDialog] = useState<"save" | "rename" | "delete" | null>(
    null
  )
  const [pending, setPending] = useState(false)
  const activeView = views.find((view) => view.id === activeViewId) ?? null

  const run = async (work: () => Promise<void>, errorMessage: string) => {
    setPending(true)
    try {
      await work()
      setDialog(null)
    } catch (error) {
      toast.danger(extractErrorMessage(error, errorMessage))
    } finally {
      setPending(false)
    }
  }

  return (
    <>
      <div className="flex items-center">
        {views.length > 0 ? (
          <Select
            aria-label="View"
            selectedKey={activeViewId}
            onSelectionChange={(key) => {
              if (key !== null) onSelectView(String(key))
            }}
            className="min-w-0"
          >
            <Select.Trigger className="flex h-8 max-w-44 items-center gap-1.5 rounded-l-lg rounded-r-none border border-r-0 border-border px-2 text-xs text-muted transition-colors hover:bg-default">
              <AppIcon icon="layout-grid" className="h-3.5 w-3.5" />
              <span className="truncate text-foreground">
                {activeView?.name ?? "View"}
              </span>
              <Select.Indicator />
            </Select.Trigger>
            <Select.Popover className="min-w-48 rounded-xl p-1">
              <ListBox>
                {views.map((view) => (
                  <ListBox.Item
                    key={view.id}
                    id={view.id ?? ""}
                    textValue={view.name ?? ""}
                  >
                    <span className="flex items-center gap-2 text-xs">
                      <AppIcon
                        icon="layout-grid"
                        className="h-3.5 w-3.5 text-muted"
                      />
                      {view.name}
                    </span>
                  </ListBox.Item>
                ))}
              </ListBox>
            </Select.Popover>
          </Select>
        ) : null}
        <Dropdown>
          <Dropdown.Trigger
            aria-label="View actions"
            className={`flex h-8 items-center gap-1 border border-border px-1.5 text-xs text-muted transition-colors hover:bg-default ${
              views.length > 0 ? "rounded-r-lg" : "rounded-lg px-2"
            }`}
          >
            {views.length > 0 ? (
              <AppIcon icon="ellipsis" className="h-3.5 w-3.5" />
            ) : (
              <>
                <AppIcon icon="layout-grid" className="h-3.5 w-3.5" />
                Views
              </>
            )}
          </Dropdown.Trigger>
          <Dropdown.Popover placement="bottom start" className="w-52">
            <Dropdown.Menu
              aria-label="View actions"
              onAction={(key) => {
                if (key === "save") setDialog("save")
                if (key === "rename") setDialog("rename")
                if (key === "delete") setDialog("delete")
              }}
            >
              <Dropdown.Item id="save" textValue="Save as view">
                <AppIcon icon="plus" className="h-3.5 w-3.5 text-muted" />
                Save as new view
              </Dropdown.Item>
              <Dropdown.Item
                id="rename"
                textValue="Rename view"
                isDisabled={!activeView}
              >
                <AppIcon icon="pencil" className="h-3.5 w-3.5 text-muted" />
                Rename view
              </Dropdown.Item>
              <Dropdown.Item
                id="delete"
                textValue="Delete view"
                variant="danger"
                isDisabled={!activeView}
              >
                <AppIcon icon="trash-2" className="h-3.5 w-3.5" />
                Delete view
              </Dropdown.Item>
            </Dropdown.Menu>
          </Dropdown.Popover>
        </Dropdown>
      </div>

      {dialog === "save" || dialog === "rename" ? (
        <ViewNameModal
          title={dialog === "save" ? "Save as view" : "Rename view"}
          submitLabel={dialog === "save" ? "Save view" : "Save"}
          initialName={
            dialog === "rename" ? (activeView?.name ?? "") : ""
          }
          pending={pending}
          onClose={() => (!pending ? setDialog(null) : null)}
          onSubmit={(name) =>
            void run(
              () =>
                dialog === "save" ? onSaveAsView(name) : onRenameView(name),
              dialog === "save"
                ? "Could not save the view"
                : "Could not rename the view"
            )
          }
        />
      ) : null}

      {dialog === "delete" ? (
        <AlertDialog>
          <AlertDialog.Backdrop
            isOpen
            onOpenChange={(open) => (!open && !pending ? setDialog(null) : null)}
            className="bg-background/80 backdrop-blur-sm"
          >
            <AlertDialog.Container placement="center" size="sm">
              <AlertDialog.Dialog className="p-8">
                <AlertDialog.Header>
                  <AlertDialog.Icon status="danger">
                    <AppIcon icon="trash-2" className="h-6 w-6" />
                  </AlertDialog.Icon>
                  <div className="flex flex-col gap-1">
                    <AlertDialog.Heading>
                      Delete “{activeView?.name ?? "view"}”
                    </AlertDialog.Heading>
                    <p className="text-sm text-muted">
                      Only the saved view configuration is removed — rows and
                      columns are untouched.
                    </p>
                  </div>
                </AlertDialog.Header>
                <AlertDialog.Footer>
                  <Button
                    variant="tertiary"
                    size="sm"
                    isDisabled={pending}
                    onPress={() => setDialog(null)}
                  >
                    Cancel
                  </Button>
                  <Button
                    variant="danger"
                    size="sm"
                    isDisabled={pending}
                    onPress={() =>
                      void run(onDeleteView, "Could not delete the view")
                    }
                  >
                    {pending ? <Spinner color="current" size="sm" /> : null}
                    Delete view
                  </Button>
                </AlertDialog.Footer>
              </AlertDialog.Dialog>
            </AlertDialog.Container>
          </AlertDialog.Backdrop>
        </AlertDialog>
      ) : null}
    </>
  )
}

function ViewNameModal({
  title,
  submitLabel,
  initialName,
  pending,
  onClose,
  onSubmit,
}: {
  title: string
  submitLabel: string
  initialName: string
  pending: boolean
  onClose: () => void
  onSubmit: (name: string) => void
}) {
  const [name, setName] = useState(initialName)
  const invalid = name.trim().length === 0

  return (
    <Modal isOpen onOpenChange={(open) => (!open ? onClose() : null)}>
      <Modal.Backdrop className="bg-background/80 backdrop-blur-sm">
        <Modal.Container placement="center" size="sm">
          <Modal.Dialog className="p-8">
            <Modal.CloseTrigger />
            <form
              onSubmit={(event) => {
                event.preventDefault()
                if (!invalid && !pending) onSubmit(name.trim())
              }}
            >
              <Modal.Header>
                <Modal.Heading>{title}</Modal.Heading>
              </Modal.Header>
              <Modal.Body>
                <TextField
                  isInvalid={invalid}
                  value={name}
                  onChange={setName}
                  className="flex flex-col gap-1.5"
                >
                  <Label>View name</Label>
                  <Input autoFocus placeholder="e.g. Active deals" />
                  <FieldError>Enter a view name.</FieldError>
                </TextField>
              </Modal.Body>
              <Modal.Footer>
                <Button
                  type="button"
                  variant="tertiary"
                  size="sm"
                  isDisabled={pending}
                  onPress={onClose}
                >
                  Cancel
                </Button>
                <Button
                  type="submit"
                  variant="primary"
                  size="sm"
                  isDisabled={invalid || pending}
                >
                  {pending ? <Spinner color="current" size="sm" /> : null}
                  {submitLabel}
                </Button>
              </Modal.Footer>
            </form>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}

/* ------------------------------------------------------------------ */
/* Hidden columns                                                      */
/* ------------------------------------------------------------------ */

function HiddenColumnsPopover({
  hiddenFields,
  onUnhideField,
}: {
  hiddenFields: SheetField[]
  onUnhideField: (fieldId: string) => void
}) {
  return (
    <Popover>
      <Popover.Trigger className="flex h-8 items-center gap-1.5 rounded-lg border border-border px-2 text-xs text-muted transition-colors hover:bg-default">
        <AppIcon icon="eye-off" className="h-3.5 w-3.5" />
        Hidden
        <span className="rounded-full bg-accent px-1.5 text-[10px] text-accent-foreground">
          {hiddenFields.length}
        </span>
      </Popover.Trigger>
      <Popover.Content className="w-64 rounded-2xl border border-border p-3">
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
}

/* ------------------------------------------------------------------ */
/* Filters                                                             */
/* ------------------------------------------------------------------ */

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
                      <Input
                        aria-label="Filter value"
                        value={rule.value}
                        onChange={(event) =>
                          updateRule(rule.id, { value: event.target.value })
                        }
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
}

/* ------------------------------------------------------------------ */
/* Sorts (ordered multi-sort)                                          */
/* ------------------------------------------------------------------ */

function SortPopover({
  fields,
  sorts,
  onChange,
}: {
  fields: SheetField[]
  sorts: SheetSort[]
  onChange: (sorts: SheetSort[]) => void
}) {
  const fieldOptions = fields
    .filter((field) => field.id)
    .map((field) => ({ value: field.id ?? "", label: field.name ?? "" }))

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
                key={`${sort.field}-${index}`}
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
}

/* ------------------------------------------------------------------ */
/* Add column                                                          */
/* ------------------------------------------------------------------ */

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
        <AppIcon icon="columns-3" className="h-3.5 w-3.5" />
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
            <AppIcon
              icon={fieldTypeIcon(type)}
              className="h-3.5 w-3.5 shrink-0 text-muted"
            />
            <ToolbarSelect
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
            <ToolbarSelect
              ariaLabel="Linked page"
              value={targetPageId}
              onChange={setTargetPageId}
              options={pages
                .filter((page) => page.id)
                .map((page) => ({
                  value: page.id ?? "",
                  label: page.name ?? "",
                }))}
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
}
