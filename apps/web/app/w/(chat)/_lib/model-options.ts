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

// Families pinned to the top of the composer model picker, in display order.
// The two most recent releases of each family lead the list.
const FAVORITE_MODEL_FAMILY_PREFIXES = [
  "glm-",
  "mimo-",
  "kimi-",
  "deepseek-",
  "qwen",
  "minimax-",
  "gemini-",
]
const FAVORITES_PER_FAMILY = 2

function isAudioOnlyInput(model: ModelSummary): boolean {
  const input = model.modalities?.input ?? []
  return input.length > 0 && input.every((mode) => mode === "audio")
}

// Model ids for the chat composer picker: audio-only-input models are
// unusable for sessions and dropped; favorite families float to the top,
// newest first; everything else keeps the API order.
export function composerModelIds(models: ModelSummary[]): string[] {
  const seen = new Set<string>()
  const usable: { id: string; releaseDate: string }[] = []
  for (const model of models) {
    const id = model.id?.trim()
    if (!id || seen.has(id) || isAudioOnlyInput(model)) continue
    seen.add(id)
    usable.push({ id, releaseDate: model.release_date ?? "" })
  }

  const favorites: string[] = []
  const favoriteIds = new Set<string>()
  for (const prefix of FAVORITE_MODEL_FAMILY_PREFIXES) {
    const family = usable
      .filter(
        (model) => model.id.startsWith(prefix) && !favoriteIds.has(model.id)
      )
      .sort((a, b) => b.releaseDate.localeCompare(a.releaseDate))
      .slice(0, FAVORITES_PER_FAMILY)
    for (const model of family) {
      favoriteIds.add(model.id)
      favorites.push(model.id)
    }
  }

  const rest = usable
    .map((model) => model.id)
    .filter((id) => !favoriteIds.has(id))
  return [...favorites, ...rest]
}
