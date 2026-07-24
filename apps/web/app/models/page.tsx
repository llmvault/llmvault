import type { Metadata } from "next"
import { CatalogPage } from "./_components/catalog-page"

export const metadata: Metadata = {
  title: "AI model catalog",
  description:
    "Compare every model available to Hivy agents, including provider routes, context windows, and per-token prices.",
}

// The catalog comes from the API configured for the running web service.
// Rendering it at build time makes the image depend on a live API endpoint.
export const dynamic = "force-dynamic"

export default function ModelsPage() {
  return <CatalogPage />
}
