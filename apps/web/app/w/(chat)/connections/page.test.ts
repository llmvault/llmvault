import React from "react"
import { renderToString } from "react-dom/server"
import { beforeEach, describe, expect, it, vi } from "vitest"

const mocks = vi.hoisted(() => ({
  push: vi.fn(),
  useMutation: vi.fn(),
  useQuery: vi.fn(),
}))

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: mocks.push }),
}))

vi.mock("@/lib/api/hooks", () => ({
  $api: {
    useMutation: mocks.useMutation,
    useQuery: mocks.useQuery,
  },
}))

vi.mock("@/lib/auth/use-role", () => ({
  useIsAdmin: () => true,
}))

vi.mock("@/components/integration-logo", () => ({
  IntegrationLogo: ({ provider }: { provider: string }) =>
    React.createElement("span", { "data-provider": provider }),
}))

vi.mock("@/components/icon", () => ({
  AppIcon: ({ icon }: { icon: string }) =>
    React.createElement("span", { "data-icon": icon }),
}))

vi.mock("./use-connect-integration", () => ({
  useConnectIntegration: () => ({
    reconnectIntegration: vi.fn(),
    connectingId: null,
  }),
}))

import ConnectionsPage from "./page"

describe("ConnectionsPage", () => {
  beforeEach(() => {
    mocks.push.mockReset()
    mocks.useMutation.mockReset()
    mocks.useMutation.mockReturnValue({
      isPending: false,
      mutateAsync: vi.fn(),
    })
    mocks.useQuery.mockReset()
    mocks.useQuery.mockImplementation((_method: string, path: string) => {
      if (path === "/v1/connections") {
        return {
          data: {
            data: [
              {
                id: "connection-1",
                display_name: "Slack",
                name: "support-slack",
                provider: "slack",
              },
              {
                id: "connection-2",
                configurable_resources: [
                  {
                    key: "repository",
                    display_name: "Repositories",
                    description: "GitHub repositories agents can access",
                  },
                ],
                display_name: "GitHub",
                meta: {},
                name: "engineering-github",
                provider: "github-app",
              },
            ],
          },
          isError: false,
          isLoading: false,
          refetch: vi.fn(),
        }
      }
      if (path === "/v1/database-integrations") {
        return {
          data: [
            {
              id: "database-1",
              display_name: "Production analytics",
              name: "analytics-db",
              provider: "postgres",
            },
          ],
          isError: false,
          isLoading: false,
          refetch: vi.fn(),
        }
      }
      return { data: undefined, isError: false, isLoading: false }
    })
  })

  it("renders existing integrations and databases as connection rows", () => {
    const html = renderToString(React.createElement(ConnectionsPage))

    expect(mocks.useQuery.mock.calls).toContainEqual([
      "get",
      "/v1/database-integrations",
    ])
    expect(mocks.useQuery.mock.calls).not.toContainEqual([
      "get",
      "/v1/integrations/available",
    ])
    expect(html).toContain("Add connection")
    expect(html).toContain("Connected")
    expect(html).toContain("support-slack")
    expect(html).toContain("Slack")
    expect(html).toContain("engineering-github")
    expect(html).toContain("Resource access is not configured")
    expect(html).toContain("Databases")
    expect(html).toContain("analytics-db")
    expect(html).toContain("PostgreSQL")
    expect(html).toContain('data-provider="slack"')
    expect(html).toContain('data-provider="github-app"')
    expect(html).toContain('data-provider="postgres"')
    expect(html).toContain('data-icon="ellipsis"')
  })
})
