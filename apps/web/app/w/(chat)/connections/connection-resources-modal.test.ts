import React from "react"
import { renderToString } from "react-dom/server"
import { beforeEach, describe, expect, it, vi } from "vitest"

const mocks = vi.hoisted(() => ({
  mutateAsync: vi.fn(),
  useMutation: vi.fn(),
  useQuery: vi.fn(),
}))

vi.mock("@/lib/api/hooks", () => ({
  $api: {
    useMutation: mocks.useMutation,
    useQuery: mocks.useQuery,
  },
}))

vi.mock("@/components/integration-logo", () => ({
  IntegrationLogo: ({ provider }: { provider: string }) =>
    React.createElement("span", { "data-provider": provider }),
}))

vi.mock("@/components/icon", () => ({
  AppIcon: ({ icon }: { icon: string }) =>
    React.createElement("span", { "data-icon": icon }),
}))

vi.mock("@heroui/react", () => {
  const part = (name: string) =>
    function Part({ children }: { children?: React.ReactNode }) {
      return React.createElement(name, null, children)
    }
  const Modal = Object.assign(part("modal"), {
    Backdrop: part("modal-backdrop"),
    Body: part("modal-body"),
    CloseTrigger: part("modal-close"),
    Container: part("modal-container"),
    Dialog: part("modal-dialog"),
    Footer: part("modal-footer"),
    Header: part("modal-header"),
    Heading: part("modal-heading"),
  })
  return {
    Button: part("button"),
    Input: (props: { value?: string; placeholder?: string }) =>
      React.createElement("input", props),
    Modal,
    Spinner: part("spinner"),
    toast: {
      danger: vi.fn(),
      success: vi.fn(),
    },
  }
})

import {
  ConnectionResourcesModal,
  connectionNeedsResourceConfiguration,
} from "./connection-resources-modal"

const githubConnection = {
  id: "connection-1",
  configurable_resources: [
    {
      key: "repository",
      display_name: "Repositories",
      description: "GitHub repositories agents can access",
    },
  ],
  display_name: "GitHub",
  meta: {
    resources: {
      repository: [
        {
          id: "usehivy/hivy",
          name: "hivy",
          type: "repository",
        },
      ],
    },
  },
  name: "engineering-github",
  provider: "github-app",
}

describe("ConnectionResourcesModal", () => {
  beforeEach(() => {
    mocks.mutateAsync.mockReset()
    mocks.useMutation.mockReset()
    mocks.useMutation.mockReturnValue({
      isPending: false,
      mutateAsync: mocks.mutateAsync,
    })
    mocks.useQuery.mockReset()
    mocks.useQuery.mockReturnValue({
      data: {
        resources: [
          { id: "usehivy/hivy", name: "hivy", type: "repository" },
          { id: "usehivy/docs", name: "docs", type: "repository" },
        ],
      },
      isError: false,
      isLoading: false,
      refetch: vi.fn(),
    })
  })

  it("loads configurable resource types and preserves saved selections", () => {
    const html = renderToString(
      React.createElement(ConnectionResourcesModal, {
        connection: githubConnection,
        onClose: vi.fn(),
        onSaved: vi.fn(),
      })
    )

    expect(mocks.useQuery).toHaveBeenCalledWith(
      "get",
      "/v1/connections/{id}/resources/{type}",
      {
        params: {
          path: {
            id: "connection-1",
            type: "repository",
          },
        },
      },
      { enabled: true, retry: false }
    )
    expect(mocks.useMutation).toHaveBeenCalledWith(
      "put",
      "/v1/connections/{id}/resources"
    )
    expect(html).toContain("Configure resources")
    expect(html).toContain("engineering-github")
    expect(html).toContain("usehivy/hivy")
    expect(html).toContain("usehivy/docs")
    expect(html).toContain("1 selected")
  })

  it("warns until every configurable resource type has a selection", () => {
    expect(
      connectionNeedsResourceConfiguration({
        ...githubConnection,
        meta: {},
      })
    ).toBe(true)
    expect(connectionNeedsResourceConfiguration(githubConnection)).toBe(false)
  })
})
