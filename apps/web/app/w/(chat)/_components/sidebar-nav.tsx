import type { ReactNode } from "react"
import { Typography } from "@heroui/react"
import { Icon } from "@iconify/react"
import { cn } from "@/lib/utils"

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
  active = false,
  className = "",
  onClick,
}: {
  icon: string
  label: string
  active?: boolean
  className?: string
  onClick?: () => void
}) {
  return (
    <button
      type="button"
      aria-current={active ? "page" : undefined}
      onClick={onClick}
      className={cn(
        "flex items-center gap-2.5 rounded-lg px-3 py-1.5 text-left text-sm transition-colors",
        active ? "bg-default text-foreground" : "hover:bg-default",
        className
      )}
    >
      <Icon
        icon={icon}
        className={cn(
          "h-4 w-4 shrink-0",
          active ? "text-foreground" : "text-muted"
        )}
      />
      <span className="min-w-0 flex-1 truncate">{label}</span>
    </button>
  )
}
