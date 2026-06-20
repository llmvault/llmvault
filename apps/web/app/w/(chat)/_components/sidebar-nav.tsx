import type { ReactNode } from "react"
import { Typography } from "@heroui/react"
import { Icon } from "@iconify/react"

export function SectionLabel({ children }: { children: ReactNode }) {
  return (
    <Typography.Paragraph
      size="xs"
      color="muted"
      className="px-3 pt-2 pb-1 select-none"
    >
      {children}
    </Typography.Paragraph>
  )
}

export function NavRow({
  icon,
  label,
  className = "",
  onClick,
}: {
  icon: string
  label: string
  className?: string
  onClick?: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`hover:bg-default flex items-center gap-2.5 rounded-lg px-3 py-1.5 text-left text-sm transition-colors ${className}`}
    >
      <Icon icon={icon} className="h-4 w-4 shrink-0 text-muted" />
      <span className="min-w-0 flex-1 truncate">{label}</span>
    </button>
  )
}
