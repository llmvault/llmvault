import { describe, expect, it } from "vitest"
import {
  assertStaticMarketingRoutes,
  staticMarketingRoutes,
} from "./static-marketing-routes.mjs"

function staticManifest() {
  return {
    routes: Object.fromEntries([
      ...staticMarketingRoutes.map((route) => [route, {}]),
      ["/blog/example", {}],
      ["/docs/example", {}],
    ]),
    dynamicRoutes: {
      "/blog/[slug]": { fallback: null },
      "/docs/[...slug]": { fallback: null },
    },
  }
}

describe("static marketing route guard", () => {
  it("accepts a build that prerenders marketing pages and collections", () => {
    expect(() => assertStaticMarketingRoutes(staticManifest())).not.toThrow()
  })

  it("rejects a build when a marketing route becomes dynamic", () => {
    const manifest = staticManifest()
    delete manifest.routes["/pricing"]

    expect(() => assertStaticMarketingRoutes(manifest)).toThrow(
      "Marketing routes must be prerendered: /pricing"
    )
  })

  it("rejects a build when the retired home route returns", () => {
    const manifest = staticManifest()
    manifest.routes["/home"] = {}

    expect(() => assertStaticMarketingRoutes(manifest)).toThrow(
      "Retired marketing routes must stay removed: /home"
    )
  })
})
