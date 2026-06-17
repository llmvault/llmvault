import { describe, expect, it } from "vitest"
import {
  pluginHasMissingResourceRequirements,
  pluginMissingResourceRequirements,
  pluginResourceSelectLabel,
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

  it("detects missing configurable resources for installed plugins", () => {
    const plugin: ApiPlugin = {
      slug: "github",
      installed: true,
      resource_requirements: [
        {
          provider: "github-app",
          kind: "integration",
          connection_id: "conn-1",
          resource_key: "repository",
          display_name: "Repositories",
          missing: true,
        },
      ],
    }

    expect(pluginHasMissingResourceRequirements(plugin)).toBe(true)
    expect(pluginMissingResourceRequirements(plugin)).toEqual([
      {
        provider: "github-app",
        kind: "integration",
        connection_id: "conn-1",
        resource_key: "repository",
        display_name: "Repositories",
        missing: true,
      },
    ])
    expect(
      pluginResourceSelectLabel(plugin.resource_requirements?.[0] ?? {})
    ).toBe("Select repositories")
  })
})
