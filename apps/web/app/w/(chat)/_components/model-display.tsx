import type { ComponentType } from "react"
import { Icon } from "@iconify/react"
import { modelById } from "@/app/w/(chat)/_lib/agents"

export type DisplayModel = {
  id: string
  label: string
  provider: string
  Icon?: ComponentType<{ className?: string }>
}

export function displayModel(id: string): DisplayModel {
  try {
    return modelById(id)
  } catch {
    return {
      id,
      label: id,
      provider: "Agent model",
    }
  }
}

export function ModelIcon({
  model,
  className = "h-4 w-4 shrink-0",
}: {
  model: DisplayModel
  className?: string
}) {
  const IconComponent = model.Icon
  if (IconComponent) {
    return <IconComponent className={className} />
  }
  return <Icon icon="lucide:brain" className={`${className} text-muted`} />
}
