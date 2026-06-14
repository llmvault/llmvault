import { useState } from "react"
import { Popover, Switch } from "@heroui/react"
import { Icon } from "@iconify/react"

export function SettingRow({
  title,
  description,
  children,
  last,
}: {
  title: string
  description: string
  children: React.ReactNode
  last?: boolean
}) {
  return (
    <div
      className={`flex items-center gap-4 px-4 py-3.5 ${last ? "" : "border-b border-border"}`}
    >
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="text-sm font-medium">{title}</span>
        <span className="text-sm text-muted">{description}</span>
      </div>
      <div className="shrink-0">{children}</div>
    </div>
  )
}

export function ToggleRow({
  title,
  description,
  learnMore,
  defaultSelected,
  last,
}: {
  title: string
  description: string
  learnMore?: boolean
  defaultSelected?: boolean
  last?: boolean
}) {
  return (
    <div
      className={`flex items-start gap-4 px-4 py-4 ${last ? "" : "border-b border-border"}`}
    >
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="text-sm font-medium">{title}</span>
        <span className="text-sm leading-6 text-muted">
          {description}{" "}
          {learnMore ? (
            <a href="#" className="text-accent underline-offset-2 hover:underline">
              Learn more
            </a>
          ) : null}
        </span>
      </div>
      <PlainSwitch defaultSelected={defaultSelected} />
    </div>
  )
}

export function PlainSwitch({ defaultSelected }: { defaultSelected?: boolean }) {
  return (
    <Switch defaultSelected={defaultSelected} className="shrink-0">
      <Switch.Control>
        <Switch.Thumb />
      </Switch.Control>
    </Switch>
  )
}

export function ControlledSwitch({
  selected,
  onChange,
}: {
  selected: boolean
  onChange: (selected: boolean) => void
}) {
  return (
    <Switch isSelected={selected} onChange={onChange} className="shrink-0">
      <Switch.Control>
        <Switch.Thumb />
      </Switch.Control>
    </Switch>
  )
}

export function Segmented({
  options,
  value,
  onChange,
}: {
  options: string[]
  value: string
  onChange: (value: string) => void
}) {
  return (
    <div className="flex items-center gap-1">
      {options.map((option) => (
        <button
          key={option}
          type="button"
          onClick={() => onChange(option)}
          className={`rounded-lg px-3 py-1 text-sm transition-colors ${
            option === value
              ? "bg-default font-medium"
              : "text-muted hover:text-foreground"
          }`}
        >
          {option}
        </button>
      ))}
    </div>
  )
}

export function IconSegmented({
  options,
  value,
  onChange,
}: {
  options: { id: string; label: string; icon: string }[]
  value: string
  onChange: (value: string) => void
}) {
  return (
    <div className="flex items-center gap-1">
      {options.map((option) => (
        <button
          key={option.id}
          type="button"
          onClick={() => onChange(option.id)}
          className={`flex items-center gap-1.5 rounded-lg px-2.5 py-1 text-sm transition-colors ${
            option.id === value
              ? "bg-default font-medium"
              : "text-muted hover:text-foreground"
          }`}
        >
          <Icon icon={option.icon} className="h-4 w-4" />
          {option.label}
        </button>
      ))}
    </div>
  )
}

export function SettingSelect({
  icon,
  options,
}: {
  icon?: string
  options: string[]
}) {
  const [open, setOpen] = useState(false)
  const [value, setValue] = useState(options[0])

  return (
    <Popover isOpen={open} onOpenChange={setOpen}>
      <Popover.Trigger
        aria-label={`Select ${value}`}
        className="flex items-center gap-2 rounded-xl border border-border bg-field-background px-3 py-1.5 text-sm transition-colors hover:bg-default"
      >
        {icon ? <Icon icon={icon} className="h-4 w-4" /> : null}
        {value}
        <Icon icon="lucide:chevron-down" className="h-3.5 w-3.5 text-muted" />
      </Popover.Trigger>
      <Popover.Content className="w-48 rounded-2xl border border-border p-1.5">
        <Popover.Dialog className="flex w-full flex-col gap-0.5 p-0">
          {options.map((option) => (
            <button
              key={option}
              type="button"
              onClick={() => {
                setValue(option)
                setOpen(false)
              }}
              className="flex items-center gap-2 rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors hover:bg-default"
            >
              <span className="min-w-0 flex-1">{option}</span>
              {option === value ? (
                <Icon icon="lucide:check" className="h-4 w-4 shrink-0" />
              ) : null}
            </button>
          ))}
        </Popover.Dialog>
      </Popover.Content>
    </Popover>
  )
}

export function NumberField({
  value,
  onChange,
  unit,
}: {
  value: number
  onChange: (value: number) => void
  unit: string
}) {
  return (
    <div className="flex items-center gap-1.5">
      <input
        type="number"
        value={value}
        aria-label="Size"
        onChange={(event) => onChange(Number(event.target.value))}
        className="w-16 rounded-lg border border-border bg-field-background px-2 py-1 text-right text-sm outline-none focus:border-accent"
      />
      <span className="text-sm text-muted">{unit}</span>
    </div>
  )
}

export function WorkModeCard({
  icon,
  title,
  description,
  selected,
  onSelect,
}: {
  icon: string
  title: string
  description: string
  selected: boolean
  onSelect: () => void
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      className={`flex items-center gap-3 rounded-2xl border px-4 py-4 text-left transition-colors ${
        selected ? "border-accent bg-surface" : "border-border bg-surface hover:bg-default"
      }`}
    >
      <Icon icon={icon} className="h-5 w-5 shrink-0 text-muted" />
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="text-sm font-medium">{title}</span>
        <span className="text-sm text-muted">{description}</span>
      </span>
      <span
        className={`flex h-4 w-4 shrink-0 items-center justify-center rounded-full border ${
          selected ? "border-accent" : "border-border"
        }`}
      >
        {selected ? <span className="h-2 w-2 rounded-full bg-accent" /> : null}
      </span>
    </button>
  )
}
