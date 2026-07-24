import "server-only"

import { cache } from "react"
import createClient from "openapi-fetch"
import type { paths } from "@/lib/api/schema"
import type { CatalogModel } from "./catalog-data"

function catalogAPIURL(): string {
  const value = process.env.HIVY_API_URL?.trim()
  if (!value) {
    throw new Error("HIVY_API_URL is required to build the model catalog")
  }
  return value.replace(/\/$/, "")
}

export async function fetchModelCatalog(
  baseURL = catalogAPIURL()
): Promise<CatalogModel[]> {
  const client = createClient<paths>({ baseUrl: baseURL })
  const { data, response } = await client.GET("/v1/catalog/models", {
    cache: "force-cache",
  })
  if (!data) {
    throw new Error(
      `Model catalog request failed with status ${response.status}`
    )
  }
  return data.models ?? []
}

export const loadModelCatalog = cache(fetchModelCatalog)
