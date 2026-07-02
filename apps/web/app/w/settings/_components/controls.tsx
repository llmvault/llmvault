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
