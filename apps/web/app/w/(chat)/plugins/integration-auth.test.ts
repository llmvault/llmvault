import { describe, expect, it } from "vitest"
import {
  type AvailableIntegration,
  integrationConnectionFields,
  integrationNeedsForm,
} from "@/app/w/(chat)/plugins/integration-auth"

describe("plugin integration auth helpers", () => {
  it("requires a form for provider-defined API key config fields", () => {
    const integration = integrationFixture({
      auth_mode: "API_KEY",
      connection_config: {
        baseUrl: {
          title: "Bugsink URL",
          description: "Base URL for the Bugsink instance.",
          type: "string",
          example: "https://bugsink.example.com",
        },
      },
    })

    expect(integrationNeedsForm(integration)).toBe(true)
    expect(integrationConnectionFields(integration)).toEqual([
      {
        key: "baseUrl",
        title: "Bugsink URL",
        description: "Base URL for the Bugsink instance.",
        placeholder: "https://bugsink.example.com",
        pattern: undefined,
      },
    ])
  })

  it("uses direct Nango auth for OAuth integrations without user fields", () => {
    const integration = integrationFixture({ auth_mode: "OAUTH2" })

    expect(integrationNeedsForm(integration)).toBe(false)
    expect(integrationConnectionFields(integration)).toEqual([])
  })

  it("uses direct Nango auth for GitHub app automated fields", () => {
    const integration = integrationFixture({
      auth_mode: "APP",
      connection_config: {
        appPublicLink: {
          title: "GitHub app link",
          description: "GitHub app public link.",
          type: "string",
          automated: true,
        },
        installation_id: {
          title: "Installation ID",
          description: "GitHub app installation id.",
          type: "string",
          automated: true,
        },
      },
    })

    expect(integrationNeedsForm(integration)).toBe(false)
    expect(integrationConnectionFields(integration)).toEqual([])
  })

  it("requires a form for outbound installs", () => {
    const integration = integrationFixture({
      auth_mode: "APP",
      installation: "outbound",
    })

    expect(integrationNeedsForm(integration)).toBe(true)
  })
})

function integrationFixture(
  nangoConfig: NonNullable<AvailableIntegration["nango_config"]>
): AvailableIntegration {
  return {
    id: "integration-1",
    provider: "bugsink",
    display_name: "Bugsink",
    created_at: "2026-01-01T00:00:00Z",
    nango_config: nangoConfig,
  }
}
