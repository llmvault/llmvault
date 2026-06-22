import { afterEach, describe, expect, it, vi } from "vitest"
import {
  useSessionRuntimeStore,
  sessionRuntimeSummary,
} from "@/app/w/(chat)/_stores/session-runtime-store"
import type { GoSessionStreamFrame } from "@/app/w/(chat)/_lib/go-session-stream"
import type { SessionEventResponse } from "@/app/w/(chat)/_lib/session-history"

describe("session runtime store", () => {
  afterEach(() => {
    useSessionRuntimeStore.setState({
      statusBySessionId: {},
      liveEventsBySessionId: {},
      subagentRunsBySessionId: {},
      cursorBySessionId: {},
      reconnectAttemptsBySessionId: {},
    })
    vi.unstubAllGlobals()
  })

  it("hydrates active session responses into streaming status", () => {
    useSessionRuntimeStore.getState().hydrateSession({
      id: "session-1",
      agent_turn_status: "active",
    })

    expect(sessionRuntimeSummary("session-1").status).toBe("streaming")
  })

  it("marks request-user-input frames as waiting for the user", () => {
    useSessionRuntimeStore.getState().applyStreamFrame(
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
    useSessionRuntimeStore.getState().applyStreamFrame(
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

  it("preserves live final and tool events when the stream finishes before history catches up", () => {
    const liveEvents = [
      sessionEvent("tool-call-1", "tool_call", 4, {
        tool_call_id: "tool-1",
        name: "shell",
      }),
      sessionEvent("final-1", "final", 5, {
        text: "Done.",
      }),
    ]
    useSessionRuntimeStore.getState().setLiveEvents("session-1", liveEvents)
    useSessionRuntimeStore.getState().setStatus("session-1", "streaming")

    useSessionRuntimeStore
      .getState()
      .finishStream("session-1", { outcome: "completed" })

    expect(sessionRuntimeSummary("session-1")).toMatchObject({
      status: "idle",
      lastOutcome: "completed",
    })
    expect(
      useSessionRuntimeStore.getState().liveEventsBySessionId["session-1"]
    ).toEqual(liveEvents)
  })

  it("reconciles live events after matching durable history arrives", () => {
    const final = sessionEvent("final-1", "final", 5, {
      text: "Done.",
    })
    useSessionRuntimeStore.getState().setLiveEvents("session-1", [final])

    useSessionRuntimeStore.getState().reconcileLiveEvents("session-1", [final])

    expect(
      useSessionRuntimeStore.getState().liveEventsBySessionId["session-1"]
    ).toEqual([])
  })

  it("keeps merged live text while durable history is still behind", () => {
    useSessionRuntimeStore.getState().applyStreamFrame(
      "session-1",
      frame("token", {
        event_id: "token-1",
        sequence: 1,
        stream_id: "stream-1",
        turn_id: "turn-1",
        text: "Hello",
      })
    )
    useSessionRuntimeStore.getState().applyStreamFrame(
      "session-1",
      frame("token", {
        event_id: "token-2",
        sequence: 2,
        stream_id: "stream-1",
        turn_id: "turn-1",
        text: " world",
      })
    )

    useSessionRuntimeStore.getState().reconcileLiveEvents("session-1", [
      sessionEvent("token-1", "token", 1, {
        turn_id: "turn-1",
        text: "Hello",
      }),
    ])

    expect(
      useSessionRuntimeStore.getState().liveEventsBySessionId["session-1"]?.[0]
        ?.payload
    ).toMatchObject({ text: "Hello world" })

    useSessionRuntimeStore.getState().reconcileLiveEvents("session-1", [
      sessionEvent("token-1", "token", 2, {
        turn_id: "turn-1",
        text: "Hello world",
      }),
    ])

    expect(
      useSessionRuntimeStore.getState().liveEventsBySessionId["session-1"]
    ).toEqual([])
  })

  it("does not treat session_waiting frames as pending user input", () => {
    useSessionRuntimeStore.getState().setStatus("session-1", "streaming")
    useSessionRuntimeStore.getState().applyStreamFrame(
      "session-1",
      frame("session_waiting", {
        event_id: "waiting-1",
        reason: "subagent_tasks",
        sequence: 2,
      })
    )

    expect(sessionRuntimeSummary("session-1").status).toBe("streaming")
    expect(sessionRuntimeSummary("session-1").pendingInput).toBeUndefined()
  })

  it("segregates subagent frames from parent live events", () => {
    useSessionRuntimeStore.getState().applyStreamFrame(
      "session-1",
      frame("token", {
        event_id: "subagent-token-1",
        sequence: 3,
        stream_id: "stream-1",
        turn_id: "turn-1",
        text: "child output",
        scope: "subagent",
        subagent: {
          job_id: "job-1",
          agent_name: "codebase-explorer",
          parent_session_id: "session-1",
          child_session_id: "subagent-job-1",
        },
      })
    )

    const state = useSessionRuntimeStore.getState()
    expect(state.liveEventsBySessionId["session-1"]).toBeUndefined()
    const run = state.subagentRunsBySessionId["session-1"].find(
      (item) => item.jobId === "job-1"
    )
    expect(run).toMatchObject({
      jobId: "job-1",
      agentName: "codebase-explorer",
      childSessionId: "subagent-job-1",
      status: "running",
    })
    expect(run?.events).toHaveLength(1)
  })

  it("does not let subagent terminal frames finish the parent session", () => {
    useSessionRuntimeStore.getState().setStatus("session-1", "streaming")
    useSessionRuntimeStore.getState().applyStreamFrame(
      "session-1",
      frame("turn_completed", {
        event_id: "subagent-turn-completed-1",
        sequence: 4,
        scope: "subagent",
        subagent: {
          job_id: "job-1",
          agent_name: "oracle",
          parent_session_id: "session-1",
          child_session_id: "subagent-job-1",
        },
      })
    )

    const state = useSessionRuntimeStore.getState()
    expect(sessionRuntimeSummary("session-1").status).toBe("streaming")
    expect(state.subagentRunsBySessionId["session-1"][0].status).toBe(
      "completed"
    )
  })

  it("keeps completed subagent runs closed when late child frames arrive", () => {
    useSessionRuntimeStore.getState().applyStreamFrame(
      "session-1",
      frame("turn_completed", {
        event_id: "subagent-turn-completed-1",
        sequence: 4,
        occurred_at: "2026-06-21T09:00:00Z",
        scope: "subagent",
        subagent: {
          job_id: "job-1",
          child_session_id: "subagent-job-1",
        },
      })
    )

    useSessionRuntimeStore.getState().applyStreamFrame(
      "session-1",
      frame("token", {
        event_id: "subagent-token-late-1",
        sequence: 5,
        stream_id: "stream-1",
        turn_id: "turn-1",
        text: "late child output",
        scope: "subagent",
        subagent: {
          job_id: "job-1",
          child_session_id: "subagent-job-1",
        },
      })
    )

    const run =
      useSessionRuntimeStore.getState().subagentRunsBySessionId["session-1"][0]
    expect(run.status).toBe("completed")
    expect(run.completedAt).toBe("2026-06-21T09:00:00Z")
  })

  it("keeps the subagent run list stable when parent frames arrive", () => {
    useSessionRuntimeStore.getState().applyStreamFrame(
      "session-1",
      frame("token", {
        event_id: "subagent-token-1",
        sequence: 3,
        stream_id: "stream-1",
        turn_id: "turn-1",
        text: "child output",
        scope: "subagent",
        subagent: {
          job_id: "job-1",
          child_session_id: "subagent-job-1",
        },
      })
    )
    const runs =
      useSessionRuntimeStore.getState().subagentRunsBySessionId["session-1"]

    useSessionRuntimeStore.getState().applyStreamFrame(
      "session-1",
      frame("token", {
        event_id: "parent-token-1",
        sequence: 4,
        stream_id: "stream-1",
        turn_id: "turn-1",
        text: "parent output",
      })
    )

    expect(
      useSessionRuntimeStore.getState().subagentRunsBySessionId["session-1"]
    ).toBe(runs)
  })

  it("does not dispatch subagent debug events unless enabled", () => {
    const dispatchEvent = vi.fn()
    vi.stubGlobal("window", {
      dispatchEvent,
      localStorage: {
        getItem: vi.fn(() => null),
      },
    })

    useSessionRuntimeStore.getState().applyStreamFrame(
      "session-1",
      frame("token", {
        event_id: "subagent-token-1",
        sequence: 3,
        stream_id: "stream-1",
        turn_id: "turn-1",
        text: "child output",
        scope: "subagent",
        subagent: {
          job_id: "job-1",
          child_session_id: "subagent-job-1",
        },
      })
    )

    expect(dispatchEvent).not.toHaveBeenCalled()
  })

  it("dispatches subagent debug events after state is updated", () => {
    let observedJobId: string | undefined
    vi.stubGlobal(
      "CustomEvent",
      class TestCustomEvent<T> {
        detail?: T
        type: string

        constructor(type: string, init?: CustomEventInit<T>) {
          this.type = type
          this.detail = init?.detail
        }
      }
    )
    vi.stubGlobal("window", {
      localStorage: {
        getItem: vi.fn(() => "1"),
      },
      dispatchEvent: vi.fn(() => {
        observedJobId =
          useSessionRuntimeStore.getState().subagentRunsBySessionId[
            "session-1"
          ]?.[0]?.jobId
        return true
      }),
    })

    useSessionRuntimeStore.getState().applyStreamFrame(
      "session-1",
      frame("token", {
        event_id: "subagent-token-1",
        sequence: 3,
        stream_id: "stream-1",
        turn_id: "turn-1",
        text: "child output",
        scope: "subagent",
        subagent: {
          job_id: "job-1",
          child_session_id: "subagent-job-1",
        },
      })
    )

    expect(observedJobId).toBe("job-1")
  })

  it("does not classify a session_id prefix alone as a subagent frame", () => {
    useSessionRuntimeStore.getState().applyStreamFrame(
      "session-1",
      frame("token", {
        event_id: "parent-token-1",
        sequence: 1,
        stream_id: "stream-1",
        turn_id: "turn-1",
        session_id: "subagent-looking-parent",
        text: "parent output",
      })
    )

    const state = useSessionRuntimeStore.getState()
    expect(state.subagentRunsBySessionId["session-1"]).toBeUndefined()
    expect(state.liveEventsBySessionId["session-1"]).toHaveLength(1)
  })
})

function frame(
  event: string,
  data: Record<string, unknown>
): GoSessionStreamFrame {
  return {
    sessionId: "session-1",
    event,
    id: `${event}-1`,
    data,
  }
}

function sessionEvent(
  id: string,
  eventType: string,
  sequence: number,
  payload: Record<string, unknown>
): SessionEventResponse {
  return {
    id,
    session_id: "session-1",
    event_id: id,
    event_type: eventType,
    sequence_number: sequence,
    payload,
    event_at: `2026-06-20T10:00:0${sequence}.000Z`,
  } as SessionEventResponse
}
