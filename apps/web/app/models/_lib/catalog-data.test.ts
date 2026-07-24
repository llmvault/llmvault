import { describe, expect, it } from "vitest"
import {
  catalogProviders,
  defaultProvider,
  filterCatalogModels,
  formatModelPrice,
  formatTokenLimit,
  modelInputPrice,
  providerForModel,
  type CatalogModel,
} from "./catalog-data"

const models: CatalogModel[] = [
  {
    id: "deepseek-v4-pro",
    name: "DeepSeek V4 Pro",
    family: "deepseek",
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
        cost: { input: 1.68, output: 3.36 },
      },
    ],
  },
  {
    id: "engy-qwen3.6-35b-a3b",
    name: "Engy Qwen3.6 35B A3B",
    family: "qwen",
    providers: [
      {
        id: "engy",
        name: "Engy",
        default: true,
        priority: 1,
        cost: { input: 0.045, output: 0.3 },
      },
    ],
  },
]

describe("model catalog data", () => {
  it("searches model identity, family, and provider fields", () => {
    expect(filterCatalogModels(models, "atlas")).toEqual([models[0]])
    expect(filterCatalogModels(models, "qwen")).toEqual([models[1]])
    expect(filterCatalogModels(models, "deepseek-v4")).toEqual([models[0]])
  })

  it("lists providers and filters models by every supporting route", () => {
    expect(catalogProviders(models)).toEqual([
      { id: "atlascloud", name: "Atlas Cloud" },
      { id: "engy", name: "Engy" },
      { id: "novita", name: "Novita AI" },
    ])
    expect(filterCatalogModels(models, "", "atlascloud")).toEqual([models[0]])
    expect(filterCatalogModels(models, "deepseek", "atlascloud")).toEqual([
      models[0],
    ])
    expect(filterCatalogModels(models, "qwen", "atlascloud")).toEqual([])
  })

  it("uses the default provider for displayed pricing", () => {
    expect(defaultProvider(models[0])?.id).toBe("novita")
    expect(modelInputPrice(models[0])).toBe(1.6)
    expect(providerForModel(models[0], "atlascloud")?.id).toBe("atlascloud")
    expect(modelInputPrice(models[0], "atlascloud")).toBe(1.68)
  })

  it("formats small prices and token limits without hiding precision", () => {
    expect(formatModelPrice(0)).toBe("$0")
    expect(formatModelPrice(0.0034)).toBe("$0.0034")
    expect(formatModelPrice(0.168)).toBe("$0.168")
    expect(formatTokenLimit(1_000_000)).toBe("1M")
    expect(formatTokenLimit(262_144)).toBe("262.1K")
  })
})
