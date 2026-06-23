import Image from "next/image"
import { Icon } from "@iconify/react"
import { modelById } from "@/app/w/(chat)/_lib/agents"
import { modelLogoURL } from "@/lib/model-logos"
import { cn } from "@/lib/utils"

export type DisplayModel = {
  id: string
  label: string
  provider: string
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
  const logoURL = modelLogoURL(model.id)
  if (logoURL) {
    return (
      <span
        className={cn(
          "flex shrink-0 items-center justify-center rounded-md bg-white p-0.5",
          className
        )}
      >
        <Image
          src={logoURL}
          alt=""
          width={16}
          height={16}
          className="size-full object-contain"
        />
      </span>
    )
  }
  return <Icon icon="lucide:brain" className={`${className} text-muted`} />
}
