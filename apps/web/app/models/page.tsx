import type { Metadata } from "next"
import { CatalogPage } from "./_components/catalog-page"

export const metadata: Metadata = {
  title: "AI model catalog",
  description:
    "Compare every model available to Hivy agents, including provider routes, context windows, and per-token prices.",
}

export const dynamic = "force-static"
export const revalidate = 3600

export default function ModelsPage() {
  return <CatalogPage />
}
