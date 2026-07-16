import React from "react"
import { renderToString } from "react-dom/server"
import { beforeEach, describe, expect, it, vi } from "vitest"

const mocks = vi.hoisted(() => ({
  useQuery: vi.fn(),
}))

vi.mock("@/lib/api/hooks", () => ({
  $api: { useQuery: mocks.useQuery },
}))

vi.mock("@/lib/auth/use-role", () => ({
  useIsAdmin: () => true,
}))

vi.mock("@/components/integration-logo", () => ({
  IntegrationLogo: ({ provider }: { provider: string }) =>
    React.createElement("span", { "data-provider": provider }),
}))

vi.mock("../database-connection-modal-content", () => ({
  DatabaseConnectionModalContent: () => null,
}))

vi.mock("../integration-credentials-form", () => ({
  IntegrationCredentialsForm: () => null,
}))

vi.mock("../use-connect-integration", () => ({
  useConnectIntegration: () => ({
    connectIntegration: vi.fn(),
    connectingId: null,
    isConnecting: false,
  }),
}))

import AddConnectionPage from "./page"

describe("AddConnectionPage", () => {
  beforeEach(() => {
    mocks.useQuery.mockReset()
    mocks.useQuery.mockReturnValue({
      data: {
        data: [
          { id: "slack", display_name: "Slack", provider: "slack" },
          { id: "notion", display_name: "Notion", provider: "notion" },
        ],
      },
      isError: false,
      isLoading: false,
    })
  })

  it("shows the complete catalog for adding a connection", () => {
    const html = renderToString(React.createElement(AddConnectionPage))

    expect(mocks.useQuery).toHaveBeenCalledWith(
      "get",
      "/v1/integrations/available"
    )
    expect(html).toContain("Add connection")
    expect(html).toContain("Databases")
    expect(html).toContain("PostgreSQL")
    expect(html).toContain("MySQL")
    expect(html).toContain("MongoDB")
    expect(html).toContain("Redis")
    expect(html).toContain("Integrations")
    expect(html).toContain("Slack")
    expect(html).toContain("Notion")
  })
})
