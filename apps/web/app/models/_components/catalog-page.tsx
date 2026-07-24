import { LandingHeader } from "../../home/_components/landing-header"
import { LandingFooter } from "../../home/_components/landing-shared"
import { loadModelCatalog } from "../_lib/catalog-server"
import { ModelCatalogExperience } from "./model-catalog-experience"

export async function CatalogPage() {
  const models = await loadModelCatalog()
  return (
    <main className="marketing-link-scope min-h-screen bg-background text-foreground">
      <LandingHeader />
      <ModelCatalogExperience models={models} />
      <LandingFooter />
    </main>
  )
}
