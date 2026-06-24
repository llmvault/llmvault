import type { components } from "@/lib/api/schema"
import type { SidebarAgentResponse } from "@/app/w/(chat)/_lib/sidebar-data"

export type AgentCatalogResponse = components["schemas"]["agentCatalogResponse"]
export type ModelSummary = components["schemas"]["modelSummary"]

export function newSessionModelIds(
  agent: SidebarAgentResponse | undefined,
  catalog: AgentCatalogResponse[]
): string[] {
  const out: string[] = []
  const seen = new Set<string>()
  const add = (value: string | undefined | null) => {
    const model = value?.trim()
    if (!model || seen.has(model)) return
    seen.add(model)
    out.push(model)
  }

  add(agent?.model)
  for (const model of agent?.available_models ?? []) add(model)

  const catalogEntry = catalogEntryForAgent(agent, catalog)
  add(catalogEntry?.model)
  for (const model of catalogEntry?.available_models ?? []) add(model)

  return out
}

export function catalogEntryForAgent(
  agent: SidebarAgentResponse | undefined,
  catalog: AgentCatalogResponse[]
): AgentCatalogResponse | undefined {
  if (!agent) return undefined

  const agentID = agent.id?.trim()
  const catalogID = agent.catalog?.id?.trim()
  const catalogSlug = agent.catalog?.slug?.trim()

  return catalog.find((entry) => {
    if (agentID && entry.installed_agent_id?.trim() === agentID) return true
    if (catalogID && entry.id?.trim() === catalogID) return true
    return Boolean(catalogSlug && entry.slug?.trim() === catalogSlug)
  })
}

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
