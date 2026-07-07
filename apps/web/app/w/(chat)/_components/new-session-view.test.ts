import React from "react"
import { renderToString } from "react-dom/server"
import { beforeEach, describe, expect, it, vi } from "vitest"

const mocks = vi.hoisted(() => ({
  composerProps: null as null | {
    modelId: string
    modelIds: string[]
    modelSummaries: Array<{ id?: string; name?: string; provider_id?: string }>
    modelsError: boolean
    modelsLoading: boolean
    onSend: (
      text: string,
      attachments: unknown[],
      codeLineComments: unknown[],
      effort: string
    ) => boolean | Promise<boolean>
  },
  mutateAsync: vi.fn(),
  queryClient: {
    invalidateQueries: vi.fn(),
    setQueryData: vi.fn(),
  },
  useMutation: vi.fn(),
  useQuery: vi.fn(),
  watchGeneratedSessionName: vi.fn(),
}))

vi.mock("@/app/w/(chat)/_components/composer", () => ({
  Composer: (props: {
    modelId: string
    modelIds: string[]
    modelSummaries: Array<{ id?: string; name?: string; provider_id?: string }>
    modelsError: boolean
    modelsLoading: boolean
    onSend: (
      text: string,
      attachments: unknown[],
      codeLineComments: unknown[],
      effort: string
    ) => boolean | Promise<boolean>
  }) => {
    mocks.composerProps = props
    return null
  },
}))

vi.mock("@/lib/api/hooks", () => ({
  $api: {
    useMutation: mocks.useMutation,
    useQuery: mocks.useQuery,
  },
}))

// The channel-scoped agent list drives the composer's agent picker. Mock it to
// return the same single assigned agent the org list used to provide.
vi.mock("@/lib/api/channel-agents", () => ({
  useChannelAgents: () => ({
    data: {
      data: [
        {
          id: "agent-1",
          name: "Hivy",
          model: "test-model",
          catalog: { id: "catalog-1", slug: "hivy" },
        },
      ],
    },
    isError: false,
    isLoading: false,
  }),
}))

vi.mock("@tanstack/react-query", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@tanstack/react-query")>()
  return {
    ...actual,
    useQueryClient: () => mocks.queryClient,
  }
})

vi.mock("@/app/w/(chat)/_lib/session-name-updates", () => ({
  watchGeneratedSessionName: mocks.watchGeneratedSessionName,
}))

import { SessionView } from "@/app/w/(chat)/_components/new-session-view"

describe("SessionView", () => {
  beforeEach(() => {
    mocks.composerProps = null
    mocks.mutateAsync.mockReset()
    mocks.queryClient.invalidateQueries.mockReset()
    mocks.queryClient.setQueryData.mockReset()
    mocks.useMutation.mockReset()
    mocks.useQuery.mockReset()
    mocks.watchGeneratedSessionName.mockReset()

    mocks.useQuery.mockImplementation((_method: string, path: string) => {
      if (path === "/v1/channels") {
        return {
          data: {
            data: [{ id: "channel-1", name: "General", is_default: true }],
          },
          isError: false,
          isLoading: false,
        }
      }
      if (path === "/v1/agents") {
        return {
          data: {
            data: [
              {
                id: "agent-1",
                name: "Hivy",
                is_default: true,
                model: "test-model",
                catalog: { id: "catalog-1", slug: "hivy" },
              },
            ],
          },
          isError: false,
          isLoading: false,
        }
      }
      if (path === "/v1/agents/models") {
        return {
          data: [
            { id: "test-model", name: "Test Model", provider_id: "test" },
            { id: "agent-extra", name: "Agent Extra", provider_id: "test" },
            {
              id: "catalog-default",
              name: "Catalog Default",
              provider_id: "test",
            },
            {
              id: "catalog-model",
              name: "Catalog Model",
              provider_id: "test",
            },
          ],
          isError: false,
          isLoading: false,
        }
      }
      return { data: undefined, isError: false, isLoading: false }
    })
    mocks.useMutation.mockReturnValue({
      isPending: false,
      mutateAsync: mocks.mutateAsync,
    })
    mocks.mutateAsync.mockResolvedValue({
      event: {
        event_id: "event-1",
        event_type: "user.message",
        session_id: "session-1",
      },
      session: {
        agent_id: "agent-1",
        channel_id: "channel-1",
        id: "session-1",
        name: "Created from response",
      },
    })
  })

  it("redirects after create without passing create-response placeholders", async () => {
    const onSessionCreated = vi.fn()

    renderToString(React.createElement(SessionView, { onSessionCreated }))

    expect(mocks.composerProps).toBeTruthy()
    if (!mocks.composerProps) {
      throw new Error("Composer props were not captured")
    }
    const sent = await mocks.composerProps.onSend(
      "Investigate this",
      [],
      [],
      "High"
    )

    expect(sent).toBe(true)
	    expect(mocks.mutateAsync).toHaveBeenCalledWith({
	      body: {
	        agent_id: "agent-1",
        channel_id: "channel-1",
        model_definition: {
          model_id: "test-model",
          reasoning_effort: "high",
        },
        text: "Investigate this",
      },
    })
    expect(onSessionCreated).toHaveBeenCalledWith(
      "general",
      "session-1",
      undefined,
      { replace: true }
    )
    expect(mocks.queryClient.invalidateQueries).toHaveBeenCalledWith({
      queryKey: ["get", "/v1/channels"],
    })
    expect(mocks.watchGeneratedSessionName).toHaveBeenCalledWith(
      mocks.queryClient,
      expect.objectContaining({ id: "session-1" })
    )
  })

  it("passes all selectable model ids to the composer with the agent default selected", () => {
    renderToString(
      React.createElement(SessionView, { onSessionCreated: vi.fn() })
    )

    expect(mocks.composerProps?.modelId).toBe("test-model")
    expect(mocks.composerProps?.modelIds).toEqual([
      "test-model",
      "agent-extra",
      "catalog-default",
      "catalog-model",
    ])
    expect(
      mocks.composerProps?.modelSummaries.map((model) => model.id)
    ).toEqual(["test-model", "agent-extra", "catalog-default", "catalog-model"])
    expect(mocks.composerProps?.modelsLoading).toBe(false)
    expect(mocks.composerProps?.modelsError).toBe(false)
  })
})
