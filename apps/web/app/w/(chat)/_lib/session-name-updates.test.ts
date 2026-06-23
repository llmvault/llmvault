import { fetchEventSource } from "@microsoft/fetch-event-source"
import { QueryClient } from "@tanstack/react-query"
import { beforeEach, describe, expect, it, vi, type Mock } from "vitest"
import {
  chatQueryKeys,
  type SessionResponse,
} from "@/app/w/(chat)/_lib/chat-cache"
import { watchGeneratedSessionName } from "@/app/w/(chat)/_lib/session-name-updates"

vi.mock("@microsoft/fetch-event-source", () => ({
  fetchEventSource: vi.fn(),
}))

const fetchEventSourceMock = fetchEventSource as unknown as Mock

function client() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  })
}

describe("watchGeneratedSessionName", () => {
  beforeEach(() => {
    fetchEventSourceMock.mockReset()
  })

  it("patches session caches when the generated name event arrives", () => {
    const queryClient = client()
    const invalidate = vi.spyOn(queryClient, "invalidateQueries")
    const updated: SessionResponse = {
      id: "session-1",
      channel_id: "channel-1",
      name: "generated-session-name",
    }
    fetchEventSourceMock.mockImplementation(async (_url, options) => {
      options.onmessage({
        event: "session.name",
        data: JSON.stringify({ session: updated }),
      })
    })

    watchGeneratedSessionName(queryClient, {
      id: "session-1",
      channel_id: "channel-1",
      name: "Investigate naming updates",
    })

    expect(fetchEventSourceMock).toHaveBeenCalledWith(
      "/api/proxy/v1/sessions/session-1/name-updates",
      expect.objectContaining({
        credentials: "include",
        method: "GET",
      })
    )
    expect(
      queryClient.getQueryData(chatQueryKeys.session("session-1"))
    ).toEqual({
      session: updated,
    })
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: ["get", "/v1/channels"],
    })
  })
})
