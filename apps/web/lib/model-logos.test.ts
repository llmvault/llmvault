import { existsSync, readdirSync, readFileSync } from "node:fs"
import { join } from "node:path"
import { describe, expect, it } from "vitest"
import { MODEL_LOGOS, modelLogoURL } from "@/lib/model-logos"

function backendCanonicalModelIDs(): string[] {
  const registryDir = join(process.cwd(), "..", "..", "internal", "registry")
  return readdirSync(registryDir)
    .filter((name) => /^hivy_models.*\.go$/.test(name))
    .flatMap((name) => {
      const source = readFileSync(join(registryDir, name), "utf8")
      return Array.from(
        source.matchAll(/\bID:\s*"([^"]+)"/g),
        (match) => match[1]
      )
    })
}

describe("model logos", () => {
  it("maps every backend canonical model to an existing static logo", () => {
    expect(Object.keys(MODEL_LOGOS).sort()).toEqual(
      backendCanonicalModelIDs().sort()
    )

    for (const logoPath of Object.values(MODEL_LOGOS)) {
      expect(logoPath).toMatch(/^\/logos\/.+\.svg$/)
      expect(existsSync(join(process.cwd(), "public", logoPath))).toBe(true)
    }
  })

  it("does not synthesize paths for unknown models", () => {
    expect(modelLogoURL("deepseek-v4-pro")).toBe("/logos/deepseek.svg")
    expect(modelLogoURL("qwen3.7-plus")).toBe("/logos/alibaba.svg")
    expect(modelLogoURL("unknown-model")).toBeUndefined()
  })
})
