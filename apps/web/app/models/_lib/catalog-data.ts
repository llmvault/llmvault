import type { components } from "@/lib/api/schema"

export type CatalogModel = components["schemas"]["catalogModelResponse"]
export type CatalogProvider =
  components["schemas"]["catalogModelProviderResponse"]

export type CatalogProviderOption = {
  id: string
  name: string
}

export function defaultProvider(
  model: CatalogModel
): CatalogProvider | undefined {
  return (
    model.providers?.find((provider) => provider.default) ??
    model.providers?.[0]
  )
}

export function providerForModel(
  model: CatalogModel,
  providerID = ""
): CatalogProvider | undefined {
  if (providerID) {
    return model.providers?.find((provider) => provider.id === providerID)
  }
  return defaultProvider(model)
}

export function filterCatalogModels(
  models: CatalogModel[],
  query: string,
  providerID = ""
): CatalogModel[] {
  const normalized = query.trim().toLowerCase()

  return models.filter((model) => {
    if (
      providerID &&
      !model.providers?.some((provider) => provider.id === providerID)
    ) {
      return false
    }
    if (!normalized) return true

    const searchable = [
      model.id,
      model.name,
      model.family,
      model.description,
      ...(model.providers?.flatMap((provider) => [
        provider.id,
        provider.name,
        provider.model_name,
      ]) ?? []),
    ]
      .filter(Boolean)
      .join(" ")
      .toLowerCase()
    return searchable.includes(normalized)
  })
}

export function catalogProviders(
  models: CatalogModel[]
): CatalogProviderOption[] {
  const providers = new Map<string, string>()
  for (const model of models) {
    for (const provider of model.providers ?? []) {
      if (!provider.id) continue
      providers.set(provider.id, provider.name || provider.id)
    }
  }

  return Array.from(providers, ([id, name]) => ({ id, name })).sort((a, b) =>
    a.name.localeCompare(b.name)
  )
}

export function modelInputPrice(
  model: CatalogModel,
  providerID = ""
): number | undefined {
  return providerForModel(model, providerID)?.cost?.input ?? model.cost?.input
}

export function modelOutputPrice(
  model: CatalogModel,
  providerID = ""
): number | undefined {
  return providerForModel(model, providerID)?.cost?.output ?? model.cost?.output
}

export function modelCachePrice(
  model: CatalogModel,
  providerID = ""
): number | undefined {
  return (
    providerForModel(model, providerID)?.cost?.cache_read ??
    model.cost?.cache_read
  )
}

export function formatModelPrice(value: number | undefined): string {
  if (value === undefined) return "Not listed"
  if (value === 0) return "$0"
  if (value < 0.01) return `$${value.toFixed(4)}`
  if (value < 1) return `$${value.toFixed(3).replace(/0+$/, "")}`
  return `$${value.toFixed(2).replace(/\.00$/, "")}`
}

export function formatTokenLimit(value: number | undefined): string {
  if (!value) return "Not listed"
  if (value >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(value % 1_000_000 ? 1 : 0)}M`
  }
  if (value >= 1_000) {
    return `${(value / 1_000).toFixed(value % 1_000 ? 1 : 0)}K`
  }
  return String(value)
}

export function modelKind(model: CatalogModel): string {
  const output = model.modalities?.output ?? []
  if (output.includes("image")) return "Image"
  if (output.includes("audio")) return "Audio"
  return model.reasoning ? "Reasoning" : "Text"
}
