import {
  CatalogSearch,
  ModelIdentity,
  ModelMeta,
  PricePair,
  type CatalogViewProps,
} from "./catalog-primitives"
import { cn } from "@/lib/utils"
import { modelInputPrice, type CatalogModel } from "../_lib/catalog-data"

const bands = [
  {
    id: "free",
    label: "Free input",
    range: "$0",
    matches: (price: number | undefined) => price === 0,
  },
  {
    id: "lean",
    label: "Lean",
    range: "Under $1 / M",
    matches: (price: number | undefined) =>
      price !== undefined && price > 0 && price < 1,
  },
  {
    id: "standard",
    label: "Standard",
    range: "$1–$5 / M",
    matches: (price: number | undefined) =>
      price !== undefined && price >= 1 && price < 5,
  },
  {
    id: "premium",
    label: "Premium",
    range: "$5+ / M",
    matches: (price: number | undefined) => price !== undefined && price >= 5,
  },
  {
    id: "unlisted",
    label: "Price pending",
    range: "Not listed",
    matches: (price: number | undefined) => price === undefined,
  },
] as const

function modelsInBand(
  models: CatalogModel[],
  band: (typeof bands)[number],
  providerID: string
) {
  return models.filter((model) =>
    band.matches(modelInputPrice(model, providerID))
  )
}

export function ModelCatalog({
  models,
  total,
  providerCount,
  providers,
  providerID,
  onProviderChange,
  query,
  onQueryChange,
}: CatalogViewProps) {
  const populatedBands = bands
    .map((band) => ({
      ...band,
      entries: modelsInBand(models, band, providerID),
    }))
    .filter((band) => band.entries.length > 0)

  return (
    <div className="mx-auto w-[calc(100%-2rem)] max-w-[1380px] pt-12 pb-28">
      <header className="mt-5 border-b border-border pb-16">
        <div className="grid gap-10 md:grid-cols-[1.3fr_0.7fr] md:items-end">
          <h1 className="max-w-[900px] text-[clamp(2rem,4.5vw,4.5rem)] leading-[0.9] font-medium tracking-[-0.06em]">
            Save on tokens by picking the right model for the job
          </h1>
          <p className="max-w-[38ch] text-sm leading-6 text-muted md:pb-2">
            Pick from {total} models across {providerCount} providers. Assign
            the most suitable model to different teams - save on cost.
          </p>
        </div>
      </header>
      <CatalogSearch
        query={query}
        onQueryChange={onQueryChange}
        count={models.length}
        total={total}
        providers={providers}
        providerID={providerID}
        onProviderChange={onProviderChange}
        className="sticky top-0 z-20 bg-background/95 backdrop-blur-sm"
      />

      {populatedBands.map((band, index) => {
        const lastDesktopRowStartsAt =
          band.entries.length - (band.entries.length % 2 || 2)

        return (
          <section
            key={band.id}
            className={cn(
              "grid min-w-0 grid-cols-[minmax(0,1fr)] py-14 lg:grid-cols-[0.38fr_1.62fr]",
              index < populatedBands.length - 1 && "border-b border-border"
            )}
          >
            <header className="min-w-0">
              <span className="font-mono text-[10px] text-muted">
                {String(index + 1).padStart(2, "0")}
              </span>
              <h2 className="mt-3 text-3xl font-medium tracking-[-0.05em]">
                {band.label}
              </h2>
              <p className="mt-2 text-sm text-muted">{band.range}</p>
            </header>
            <div className="mt-8 grid min-w-0 grid-cols-[minmax(0,1fr)] gap-x-10 lg:mt-0 lg:grid-cols-2">
              {band.entries.map((model, modelIndex) => {
                const isFirstItem = modelIndex === 0
                const isInFirstDesktopRow = modelIndex < 2
                const isLastItem = modelIndex === band.entries.length - 1
                const isInLastDesktopRow = modelIndex >= lastDesktopRowStartsAt

                return (
                  <article
                    key={model.id}
                    className={cn(
                      "min-w-0 pb-6",
                      isFirstItem ? "pt-0" : "pt-6",
                      isInFirstDesktopRow ? "lg:pt-0" : "lg:pt-6",
                      isLastItem ? "border-b-0" : "border-b border-border",
                      isInLastDesktopRow
                        ? "lg:border-b-0"
                        : "lg:border-b lg:border-border"
                    )}
                  >
                    <ModelIdentity model={model} compact />
                    <div className="mt-5">
                      <PricePair model={model} providerID={providerID} />
                    </div>
                    <div className="mt-4">
                      <ModelMeta model={model} providerID={providerID} />
                    </div>
                  </article>
                )
              })}
            </div>
          </section>
        )
      })}
      {models.length === 0 ? (
        <p className="py-24 text-center text-sm text-muted">
          No price bands contain that search.
        </p>
      ) : null}
    </div>
  )
}
