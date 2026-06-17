import { describe, expect, it } from "vitest"
import {
  pluginShownRequiredConnections,
  type ApiPlugin,
} from "@/app/w/(chat)/plugins/_lib"

describe("plugin catalog helpers", () => {
  it("keeps required connections visible after they are connected", () => {
    const plugin: ApiPlugin = {
      slug: "github",
      required_connections: [
        { provider: "github-app", kind: "integration", required: true },
      ],
      missing_requirements: [],
    }

    expect(pluginShownRequiredConnections(plugin)).toEqual([
      { provider: "github-app", kind: "integration", required: true },
    ])
  })
})
