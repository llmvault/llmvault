"use client"

import { Button } from "@heroui/react"
import { AppIcon } from "@/components/icon"

export function CollectionState({
  icon,
  title,
  description,
  action,
}: {
  icon: string
  title: string
  description: string
  action?: {
    label: string
    icon: string
    variant: "primary" | "secondary"
    onPress: () => void
  }
}) {
  return (
    <div className="flex min-h-56 flex-col items-center justify-center px-6 py-10 text-center">
      <AppIcon
        icon={icon}
        className="text-muted-foreground size-7"
        aria-hidden="true"
      />
      <div role="status" aria-live="polite">
        <p className="mt-4 text-sm font-semibold text-foreground">{title}</p>
        <p className="text-muted-foreground mt-1 max-w-sm text-sm leading-5">
          {description}
        </p>
      </div>
      {action ? (
        <Button
          type="button"
          variant={action.variant}
          size="sm"
          className="mt-5"
          onPress={action.onPress}
        >
          <AppIcon icon={action.icon} className="size-4" />
          {action.label}
        </Button>
      ) : null}
    </div>
  )
}
