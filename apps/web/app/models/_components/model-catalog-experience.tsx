"use client"

import { useMemo, useState } from "react"
import {
  catalogProviders,
  filterCatalogModels,
  type CatalogModel,
} from "../_lib/catalog-data"
import { ModelCatalog } from "./model-catalog"

export function ModelCatalogExperience({ models }: { models: CatalogModel[] }) {
  const [query, setQuery] = useState("")
  const [providerID, setProviderID] = useState("")
  const providers = useMemo(() => catalogProviders(models), [models])
  const filtered = useMemo(
    () => filterCatalogModels(models, query, providerID),
    [models, providerID, query]
  )
  return (
    <ModelCatalog
      models={filtered}
      total={models.length}
      providerCount={providers.length}
      providers={providers}
      providerID={providerID}
      onProviderChange={setProviderID}
      query={query}
      onQueryChange={setQuery}
    />
  )
}
