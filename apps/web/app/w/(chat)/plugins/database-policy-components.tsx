"use client"

import { Button, Label, Spinner } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { cn } from "@/lib/utils"

export function ConfigureFrame({
  children,
  selectedObjectCount,
  canSave,
  saving,
  introspecting,
  canManage = true,
  errorMessage,
  onRetryIntrospection,
  onSave,
}: {
  children: React.ReactNode
  selectedObjectCount: number
  canSave: boolean
  saving: boolean
  introspecting: boolean
  canManage?: boolean
  errorMessage: string | null
  onRetryIntrospection: () => void
  onSave: () => void
}) {
  return (
    <div className="flex min-h-0 flex-col gap-4">
      {children}

      {errorMessage ? (
        <div className="flex items-center justify-between gap-3 rounded-2xl bg-danger/10 px-3 py-2 text-sm text-danger">
          <span>{errorMessage}</span>
          <Button
            type="button"
            variant="tertiary"
            size="sm"
            isDisabled={!canManage || introspecting}
            onPress={onRetryIntrospection}
          >
            {introspecting ? <Spinner color="current" size="sm" /> : null}
            Retry
          </Button>
        </div>
      ) : null}

      <div className="flex items-center justify-between pt-2">
        <p className="text-xs text-muted">
          {!canManage
            ? "Only workspace admins can save database access."
            : `${selectedObjectCount} item${
                selectedObjectCount === 1 ? "" : "s"
              } enabled`}
        </p>
        <Button
          type="button"
          variant="primary"
          size="sm"
          className="rounded-full"
          isDisabled={!canSave}
          onPress={onSave}
        >
          {saving ? <Spinner color="current" size="sm" /> : null}
          Save access
        </Button>
      </div>
    </div>
  )
}

export function TogglePill({
  selected,
  onPress,
  children,
}: {
  selected: boolean
  onPress: () => void
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onPress}
      className={cn(
        "rounded-lg px-3 py-1.5 text-sm transition-colors",
        selected
          ? "bg-primary text-primary-foreground"
          : "bg-default text-muted"
      )}
    >
      {children}
    </button>
  )
}

export function ExpandablePolicyRow({
  id,
  title,
  subtitle,
  checked,
  expanded,
  children,
  onCheckedChange,
  onToggleExpanded,
}: {
  id: string
  title: string
  subtitle: string
  checked: boolean
  expanded: boolean
  children: React.ReactNode
  onCheckedChange: () => void
  onToggleExpanded: () => void
}) {
  return (
    <div className="rounded-2xl bg-default">
      <div className="flex items-center gap-3 px-3 py-3">
        <input
          id={`database-object-${id}`}
          type="checkbox"
          checked={checked}
          onChange={onCheckedChange}
          className="size-4 accent-current"
        />
        <Label
          htmlFor={`database-object-${id}`}
          className="min-w-0 flex-1 cursor-pointer"
        >
          <span className="block truncate text-sm font-medium text-foreground">
            {title}
          </span>
          <span className="block truncate text-xs font-normal text-muted">
            {subtitle}
          </span>
        </Label>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          isIconOnly
          onPress={onToggleExpanded}
          aria-label={expanded ? "Collapse fields" : "Expand fields"}
        >
          <AppIcon
            icon={expanded ? "chevron-down" : "chevron-right"}
            className="h-4 w-4"
          />
        </Button>
      </div>
      {expanded ? <div className="px-3 pb-3">{children}</div> : null}
    </div>
  )
}

export function FieldMaskList({
  fields,
  disabled,
  threeColumns = false,
  maskedFields,
  onToggleMask,
}: {
  fields: { name: string; detail: string }[]
  disabled: boolean
  threeColumns?: boolean
  maskedFields: Set<string>
  onToggleMask: (field: string) => void
}) {
  if (fields.length === 0) {
    return <p className="text-sm text-muted">No fields inferred.</p>
  }

  return (
    <div
      className={cn(
        "grid gap-2",
        threeColumns ? "sm:grid-cols-3" : "sm:grid-cols-2"
      )}
    >
      {fields.map((field) => (
        <label
          key={field.name}
          className={cn(
            "flex items-center gap-2 rounded-lg px-2 py-1.5 text-sm",
            disabled ? "opacity-50" : "hover:bg-background"
          )}
        >
          <input
            type="checkbox"
            checked={maskedFields.has(field.name)}
            disabled={disabled}
            onChange={() => onToggleMask(field.name)}
            className="size-4 accent-current"
          />
          <span className="min-w-0 flex-1 truncate">{field.name}</span>
          {field.detail ? (
            <span className="shrink-0 text-xs text-muted">{field.detail}</span>
          ) : null}
        </label>
      ))}
    </div>
  )
}

export function EmptyPolicyState({ text }: { text: string }) {
  return (
    <div className="rounded-2xl bg-default px-3 py-8 text-center text-sm text-muted">
      {text}
    </div>
  )
}
