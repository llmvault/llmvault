import React from "react"
import { renderToString } from "react-dom/server"
import { beforeEach, describe, expect, it, vi } from "vitest"

const mocks = vi.hoisted(() => ({
  rows: [] as Array<{
    title: string
    subtitle: string
    on: boolean
    disabled: boolean
    onChange: (value: boolean) => void
  }>,
  useQuery: vi.fn(),
}))

vi.mock("@/lib/api/hooks", () => ({
  $api: { useQuery: mocks.useQuery },
}))

vi.mock("@/components/integration-logo", () => ({
  IntegrationLogo: ({ provider }: { provider: string }) =>
    React.createElement("span", { "data-provider": provider }),
}))

vi.mock("@/app/w/settings/teams/[teamId]/_provisioning-row", () => ({
  SectionHeader: ({
    title,
    description,
  }: {
    title: string
    description: string
  }) => React.createElement("header", null, title, description),
  EmptyProvisioningRow: ({ text }: { text: string }) =>
    React.createElement("p", null, text),
  ProvisioningSkeleton: () => React.createElement("p", null, "Loading"),
  ProvisioningRow: (props: (typeof mocks.rows)[number]) => {
    mocks.rows.push(props)
    return React.createElement(
      "div",
      {
        "data-disabled": props.disabled,
        "data-enabled": props.on,
      },
      props.title,
      props.subtitle
    )
  },
}))

import { AgentConnectionsField } from "./_connections-field"

describe("AgentConnectionsField", () => {
  beforeEach(() => {
    mocks.rows.length = 0
    mocks.useQuery.mockReset()
    mocks.useQuery.mockImplementation((_method: string, path: string) => {
      if (path === "/v1/connections") {
        return {
          data: {
            data: [
              {
                id: "00000000-0000-0000-0000-000000000001",
                name: "support-slack",
                provider: "slack",
              },
            ],
          },
          isLoading: false,
        }
      }
      if (path === "/v1/database-integrations") {
        return {
          data: [
            {
              id: "00000000-0000-0000-0000-000000000002",
              name: "production-db",
              provider: "postgres",
            },
          ],
          isLoading: false,
        }
      }
      if (path === "/v1/orgs/current/teams/{teamID}/connections") {
        return {
          data: {
            data: [
              { id: "00000000-0000-0000-0000-000000000001" },
              { id: "00000000-0000-0000-0000-000000000002" },
            ],
          },
          isLoading: false,
        }
      }
      return { data: undefined, isLoading: false }
    })
  })

  it("renders team-granted integrations and databases with inherited agent access", () => {
    const html = renderToString(
      React.createElement(AgentConnectionsField, {
        teamId: "team-1",
        value: {},
        disabled: false,
        onChange: vi.fn(),
      })
    )

    expect(html).toContain("Connections are inherited from the team by default")
    expect(html).toContain("support-slack")
    expect(html).toContain("production-db")
    expect(mocks.rows.map((row) => row.on)).toEqual([true, true])
  })

  it("persists an agent opt-out while required catalog connections stay locked", () => {
    const onChange = vi.fn()
    renderToString(
      React.createElement(AgentConnectionsField, {
        teamId: "team-1",
        value: {
          "00000000-0000-0000-0000-000000000002": ["query"],
        },
        disabled: false,
        lockedProviders: ["slack"],
        onChange,
      })
    )

    expect(mocks.rows[0]).toMatchObject({
      title: "support-slack",
      on: true,
      disabled: true,
    })
    mocks.rows[1].onChange(false)
    expect(onChange).toHaveBeenCalledWith({
      "00000000-0000-0000-0000-000000000002": ["*", "query"],
    })
  })
})
