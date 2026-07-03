import type { components } from "@/lib/api/schema"

export type ModelSummary = components["schemas"]["modelSummary"]

export function availableModelIds(models: ModelSummary[]): string[] {
  const out: string[] = []
  const seen = new Set<string>()
  for (const model of models) {
    const id = model.id?.trim()
    if (!id || seen.has(id)) continue
    seen.add(id)
    out.push(id)
  }
  return out
}
