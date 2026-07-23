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
  mutate: vi.fn(),
  invalidateQueries: vi.fn(),
}))

vi.mock("@/lib/api/hooks", () => ({
  $api: {
    useQuery: mocks.useQuery,
    useMutation: () => ({
      mutate: mocks.mutate,
      isPending: false,
    }),
  },
}))

vi.mock("@tanstack/react-query", () => ({
  useQueryClient: () => ({
    invalidateQueries: mocks.invalidateQueries,
  }),
}))

vi.mock("@/components/icon", () => ({
  AppIcon: ({ icon }: { icon: string }) =>
    React.createElement("span", { "data-icon": icon }),
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

import { AgentEnvironmentVariables } from "./_agent-environment-variables"

describe("AgentEnvironmentVariables", () => {
  beforeEach(() => {
    mocks.rows.length = 0
    mocks.useQuery.mockReset()
    mocks.mutate.mockReset()
    mocks.invalidateQueries.mockReset()
    mocks.useQuery.mockReturnValue({
      data: {
        data: [
          {
            name: "ANALYTICS_TOKEN",
            description: "Queries analytics data",
            enabled: true,
          },
          {
            name: "CRM_TOKEN",
            description: "",
            enabled: false,
          },
        ],
      },
      isLoading: false,
      isError: false,
    })
  })

  it("renders inherited access and disables a variable for new sessions", () => {
    const html = renderToString(
      React.createElement(AgentEnvironmentVariables, { agentId: "agent-1" })
    )

    expect(html).toContain("Variables are inherited from the team by default")
    expect(html).toContain("Changes apply to new sessions")
    expect(mocks.rows).toHaveLength(2)
    expect(mocks.rows[0]).toMatchObject({
      title: "ANALYTICS_TOKEN",
      subtitle: "Queries analytics data",
      on: true,
      disabled: false,
    })
    expect(mocks.rows[1]).toMatchObject({
      title: "CRM_TOKEN",
      subtitle: "Team environment variable",
      on: false,
    })

    mocks.rows[0].onChange(false)
    expect(mocks.mutate).toHaveBeenCalledWith(
      {
        params: {
          path: {
            id: "agent-1",
            name: "ANALYTICS_TOKEN",
          },
        },
        body: { enabled: false },
      },
      expect.objectContaining({
        onSuccess: expect.any(Function),
        onError: expect.any(Function),
      })
    )
  })
})
