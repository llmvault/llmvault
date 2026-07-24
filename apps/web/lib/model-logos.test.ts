import { existsSync, readdirSync, readFileSync } from "node:fs"
import { join } from "node:path"
import { describe, expect, it } from "vitest"
import { MODEL_LOGOS, modelLogoURL } from "@/lib/model-logos"
import { PROVIDER_LOGOS, providerLogoURL } from "@/lib/provider-logos"

function backendCanonicalModelIDs(): string[] {
  const registryDir = join(process.cwd(), "..", "..", "internal", "registry")
  const registrySources = readdirSync(registryDir)
    .filter((name) => name.endsWith(".go"))
    .map((name) => {
      return readFileSync(join(registryDir, name), "utf8")
    })
  const stringConstants = new Map(
    registrySources.flatMap((source) =>
      Array.from(
        source.matchAll(/\b([A-Za-z]\w*)\s*=\s*"([^"]+)"/g),
        (match) => [match[1], match[2]] as const
      )
    )
  )

  return readdirSync(registryDir)
    .filter((name) => /^hivy_models.*\.go$/.test(name))
    .flatMap((name) => {
      const source = readFileSync(join(registryDir, name), "utf8")
      return Array.from(source.matchAll(/\bID:\s*(?:"([^"]+)"|([A-Za-z]\w*))/g))
        .map((match) => match[1] ?? stringConstants.get(match[2]))
        .filter((modelID): modelID is string => Boolean(modelID))
    })
}

describe("model logos", () => {
  it("maps every backend canonical model to an existing static logo", () => {
    expect(Object.keys(MODEL_LOGOS).sort()).toEqual(
      backendCanonicalModelIDs().sort()
    )

    for (const logoPath of Object.values(MODEL_LOGOS)) {
      expect(logoPath).toMatch(/^\/logos\/.+\.(jpe?g|png|svg|webp)$/)
      expect(logoPath).not.toContain("/logos/openrouter-")
      expect(existsSync(join(process.cwd(), "public", logoPath))).toBe(true)
    }
  })

  it("does not synthesize paths for unknown models", () => {
    expect(modelLogoURL("deepseek-v4-pro")).toBe("/logos/deepseek.png")
    expect(modelLogoURL("qwen3.7-plus")).toBe("/logos/qwen.png")
    expect(modelLogoURL("unknown-model")).toBeUndefined()
  })

  it("uses model author logos for routed model owners", () => {
    expect(modelLogoURL("ling-2.6-1t")).toBe("/logos/inclusionai.png")
    expect(modelLogoURL("grok-4.3")).toBe("/logos/x-ai.png")
    expect(modelLogoURL("glm-5.2")).toBe("/logos/z-ai.png")
    expect(modelLogoURL("nemotron-3-nano-30b-a3b")).toBe("/logos/nvidia.png")
  })

  it("resolves provider aliases used by settings surfaces", () => {
    for (const logoPath of Object.values(PROVIDER_LOGOS)) {
      expect(logoPath).toMatch(/^\/logos\/.+\.(jpe?g|png|svg|webp)$/)
      expect(logoPath).not.toContain("/logos/openrouter-")
      expect(existsSync(join(process.cwd(), "public", logoPath))).toBe(true)
    }

    expect(providerLogoURL("alibaba")).toBe("/logos/alibaba.png")
    expect(providerLogoURL("byte-plus")).toBe("/logos/byteplus.png")
    expect(providerLogoURL("together")).toBe("/logos/together.png")
    expect(providerLogoURL("thegrid")).toBe("/logos/thegrid.svg")
    expect(providerLogoURL("xai")).toBe("/logos/x-ai.png")
    expect(providerLogoURL("zai")).toBe("/logos/z-ai.png")
    expect(modelLogoURL("qwen-max")).toBeUndefined()
  })
})
