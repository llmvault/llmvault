import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import { ModelCatalogExperience } from "./_components/model-catalog-experience"
import type { CatalogModel } from "./_lib/catalog-data"

const catalog: CatalogModel[] = [
  {
    id: "deepseek-v4-pro",
    name: "DeepSeek V4 Pro",
    family: "deepseek",
    reasoning: true,
    limit: { context: 1_000_000, output: 384_000 },
    modalities: { input: ["text"], output: ["text"] },
    providers: [
      {
        id: "novita",
        name: "Novita AI",
        default: true,
        priority: 1,
        cost: { input: 1.6, output: 3.2, cache_read: 0.135 },
      },
      {
        id: "atlascloud",
        name: "Atlas Cloud",
        default: false,
        priority: 2,
      },
    ],
  },
  {
    id: "ling-3.0-flash",
    name: "Ling 3.0 Flash",
    family: "ling",
    modalities: { input: ["text"], output: ["text"] },
    providers: [
      {
        id: "novita",
        name: "Novita AI",
        default: true,
        priority: 1,
        cost: { input: 0, output: 0 },
      },
    ],
  },
]

describe("model catalog", () => {
  it("server-renders searchable price bands with catalog data", () => {
    const html = renderToString(
      React.createElement(ModelCatalogExperience, {
        models: catalog,
      })
    )
    const text = html.replaceAll("<!-- -->", "")

    expect(text).toContain(
      "Save on tokens by picking the right model for the job"
    )
    expect(text).toContain("Pick from 2 models across 2 providers.")
    expect(html).toContain('aria-label="Search model catalog"')
    expect(html).toContain('aria-label="Filter models by provider"')
    expect(html).toContain("All providers")
    expect(html).toContain("Atlas Cloud")
    expect(html).toContain("DeepSeek V4 Pro")
    expect(html).toContain("Ling 3.0 Flash")
    expect(html).toContain("Novita AI")
    expect(html).toContain("$1.6")
    expect(html).toContain("$3.20")
  })
})
