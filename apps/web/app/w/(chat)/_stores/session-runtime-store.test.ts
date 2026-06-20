import { afterEach, describe, expect, it } from "vitest"
import {
  useSessionRuntimeStore,
  sessionRuntimeSummary,
} from "@/app/w/(chat)/_stores/session-runtime-store"
import type { DirectSessionStreamFrame } from "@/app/w/(chat)/_lib/direct-session-stream"

describe("session runtime store", () => {
  afterEach(() => {
    useSessionRuntimeStore.setState({
      statusBySessionId: {},
      liveEventsBySessionId: {},
      cursorBySessionId: {},
      reconnectAttemptsBySessionId: {},
    })
  })

  it("hydrates active session responses into streaming status", () => {
    useSessionRuntimeStore.getState().hydrateSession({
      id: "session-1",
      agent_turn_status: "active",
    })

    expect(sessionRuntimeSummary("session-1").status).toBe("streaming")
  })

  it("marks request-user-input frames as waiting for the user", () => {
    useSessionRuntimeStore
      .getState()
      .applyStreamFrame(
        "session-1",
        frame("question_requested", {
          request_id: "question-1",
          prompt: "Pick an option",
          turn_id: "turn-1",
        })
      )

    expect(sessionRuntimeSummary("session-1")).toMatchObject({
      status: "waiting_for_user",
      pendingInput: {
        requestId: "question-1",
        prompt: "Pick an option",
        turnId: "turn-1",
      },
    })
  })

  it("keeps sidebar status separate from live token events", () => {
    useSessionRuntimeStore
      .getState()
      .applyStreamFrame(
        "session-1",
        frame("token", {
          event_id: "token-1",
          sequence: 1,
          stream_id: "stream-1",
          turn_id: "turn-1",
          text: "Hello",
        })
      )

    expect(sessionRuntimeSummary("session-1").status).toBe("streaming")
    expect(
      useSessionRuntimeStore.getState().liveEventsBySessionId["session-1"]
    ).toHaveLength(1)
  })
})

function frame(
  event: string,
  data: Record<string, unknown>
): DirectSessionStreamFrame {
  return {
    sessionId: "session-1",
    event,
    id: `${event}-1`,
    data,
  }
}
