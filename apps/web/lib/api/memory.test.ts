import React from "react"
import { renderToString } from "react-dom/server"
import { beforeEach, describe, expect, it, vi } from "vitest"

const mocks = vi.hoisted(() => ({
  useQuery: vi.fn(),
  useInfiniteQuery: vi.fn(),
  useMutation: vi.fn(),
  invalidateQueries: vi.fn(),
}))

vi.mock("@/lib/api/hooks", () => ({
  $api: {
    useQuery: mocks.useQuery,
    useInfiniteQuery: mocks.useInfiniteQuery,
    useMutation: mocks.useMutation,
  },
}))

vi.mock("@tanstack/react-query", () => ({
  useQueryClient: () => ({ invalidateQueries: mocks.invalidateQueries }),
}))

import {
  useConfirmObservation,
  useDirectives,
  useObservations,
  usePinObservation,
  type DirectivesController,
  type ObservationsController,
} from "@/lib/api/memory"

function probe<T>(render: () => T): T {
  const captured: T[] = []
  function Probe() {
    captured.push(render())
    return null
  }
  renderToString(React.createElement(Probe))
  const result = captured[0]
  if (result === undefined) throw new Error("hook did not run")
  return result
}

beforeEach(() => {
  mocks.useQuery.mockReset()
  mocks.useInfiniteQuery.mockReset()
  mocks.useMutation.mockReset()
  mocks.invalidateQueries.mockReset()
})

describe("useObservations", () => {
  it("flattens pages and maps the snake_case response to the domain shape", () => {
    mocks.useInfiniteQuery.mockReturnValue({
      data: {
        pages: [
          {
            data: [
              {
                id: "o1",
                content: "Prefers dark mode",
                kind: "preference",
                entities: ["ui", 42],
                proof_count: 3,
                last_mentioned_at: "2026-01-02T00:00:00Z",
                expires_at: "",
                human_verified: true,
                created_at: "2026-01-01T00:00:00Z",
                archived_at: "",
              },
            ],
          },
          { data: [{ id: "o1" }, { id: "o2", content: "Second" }] },
        ],
      },
      hasNextPage: true,
      isLoading: false,
      isError: false,
      isFetchingNextPage: false,
      fetchNextPage: vi.fn(),
    })

    const result = probe<ObservationsController>(() =>
      useObservations("channel-1")
    )

    // Dedupe by id across pages: o1 appears twice, only kept once.
    expect(result.observations.map((o) => o.id)).toEqual(["o1", "o2"])
    const first = result.observations[0]!
    expect(first.proofCount).toBe(3)
    expect(first.entities).toEqual(["ui"]) // non-string entity dropped
    expect(first.humanVerified).toBe(true)
    expect(first.expiresAt).toBeNull() // "" normalized to null
    expect(result.hasMore).toBe(true)
  })

  it("defaults proofCount to 1 and disables when no channel", () => {
    mocks.useInfiniteQuery.mockReturnValue({
      data: { pages: [{ data: [{ id: "o1" }] }] },
      hasNextPage: false,
      isLoading: false,
      isError: false,
      isFetchingNextPage: false,
      fetchNextPage: vi.fn(),
    })

    probe<ObservationsController>(() => useObservations(""))
    // The 4th arg holds the react-query options; enabled follows channel id.
    expect(mocks.useInfiniteQuery.mock.calls[0]?.[3]).toMatchObject({
      enabled: false,
      pageParamName: "offset",
    })
  })

  it("computes the next offset from loaded row counts", () => {
    mocks.useInfiniteQuery.mockReturnValue({
      data: { pages: [] },
      hasNextPage: false,
      isLoading: false,
      isError: false,
      isFetchingNextPage: false,
      fetchNextPage: vi.fn(),
    })

    probe<ObservationsController>(() => useObservations("channel-1"))
    const options = mocks.useInfiniteQuery.mock.calls[0]?.[3]
    const getNext = options.getNextPageParam as (
      last: unknown,
      all: unknown[]
    ) => number | undefined
    expect(
      getNext({ has_more: true }, [{ data: [{}, {}] }, { data: [{}] }])
    ).toBe(3)
    expect(getNext({ has_more: false }, [{ data: [{}] }])).toBeUndefined()
  })
})

describe("useDirectives", () => {
  it("maps directives and coerces the source", () => {
    mocks.useQuery.mockReturnValue({
      data: {
        data: [
          {
            id: "d1",
            channel_id: "channel-1",
            content: "Reply in English",
            source: "extracted-confirmed",
            active: false,
            created_at: "2026-01-01T00:00:00Z",
          },
          { id: "d2", content: "Pinned", source: "weird-value" },
        ],
      },
      isLoading: false,
      isError: false,
    })

    const result = probe<DirectivesController>(() => useDirectives("channel-1"))
    expect(result.directives[0]).toMatchObject({
      id: "d1",
      channelId: "channel-1",
      source: "extracted-confirmed",
      active: false,
    })
    // Unknown source falls back to user-pinned; active defaults to true.
    expect(result.directives[1]).toMatchObject({
      id: "d2",
      channelId: null,
      source: "user-pinned",
      active: true,
    })
  })
})

describe("mutation hooks", () => {
  it("invalidates the channel's observation list on success", () => {
    let onSuccess: (() => void) | undefined
    mocks.useMutation.mockImplementation(
      (_method: string, _path: string, options: { onSuccess: () => void }) => {
        onSuccess = options.onSuccess
        return { mutate: vi.fn() }
      }
    )

    probe(() => useConfirmObservation("channel-1"))
    expect(mocks.useMutation.mock.calls[0]?.[1]).toBe(
      "/v1/observations/{id}/confirm"
    )
    onSuccess?.()
    expect(mocks.invalidateQueries).toHaveBeenCalledWith({
      queryKey: [
        "get",
        "/v1/observations",
        { params: { query: { channel_id: "channel-1" } } },
      ],
    })
  })

  it("pinning invalidates both observations and directives", () => {
    let onSuccess: (() => void) | undefined
    mocks.useMutation.mockImplementation(
      (_method: string, _path: string, options: { onSuccess: () => void }) => {
        onSuccess = options.onSuccess
        return { mutate: vi.fn() }
      }
    )

    probe(() => usePinObservation("channel-1"))
    onSuccess?.()
    const keys = mocks.invalidateQueries.mock.calls.map(
      (call) => call[0].queryKey[1]
    )
    expect(keys).toEqual(["/v1/observations", "/v1/directives"])
  })
})
