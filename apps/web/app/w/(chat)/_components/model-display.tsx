import Image from "next/image"
import { Icon } from "@iconify/react"
import { modelById } from "@/app/w/(chat)/_lib/agents"
import type { ModelSummary } from "@/app/w/(chat)/_lib/model-options"
import { modelLogoURL } from "@/lib/model-logos"
import { cn } from "@/lib/utils"

export type DisplayModel = {
  id: string
  label: string
  provider: string
}

export function displayModel(
  id: string,
  summaries?: ModelSummary[] | Map<string, ModelSummary>
): DisplayModel {
  const summary = modelSummaryForId(id, summaries)
  if (summary) return displayModelSummary(id, summary)

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

export function displayModelSummary(
  id: string,
  summary: ModelSummary
): DisplayModel {
  return {
    id,
    label: summary.name?.trim() || id,
    provider: modelProviderLabel(summary),
  }
}

function modelSummaryForId(
  id: string,
  summaries?: ModelSummary[] | Map<string, ModelSummary>
): ModelSummary | undefined {
  if (!summaries) return undefined
  if (summaries instanceof Map) return summaries.get(id)
  return summaries.find((entry) => entry.id?.trim() === id)
}

function modelProviderLabel(summary: ModelSummary): string {
  if (summary.provider_id?.trim()) return summary.provider_id.trim()
  const providers = summary.provider_ids
    ?.map((provider) => provider.trim())
    .filter(Boolean)
  if (providers?.length) return providers.join(", ")
  return summary.family?.trim() || "Model"
}

export function ModelIcon({
  model,
  className = "h-6 w-6 shrink-0",
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
          width={24}
          height={24}
          className="size-full object-contain"
        />
      </span>
    )
  }
  return <Icon icon="lucide:brain" className={`${className} text-muted`} />
}
