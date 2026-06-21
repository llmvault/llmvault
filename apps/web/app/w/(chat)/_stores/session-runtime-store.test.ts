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
      subagentRunsBySessionId: {},
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
    expect(state.subagentRunsBySessionId["session-1"]["job-1"]).toMatchObject({
      jobId: "job-1",
      agentName: "codebase-explorer",
      childSessionId: "subagent-job-1",
      status: "running",
    })
    expect(
      state.subagentRunsBySessionId["session-1"]["job-1"].events
    ).toHaveLength(1)
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
    expect(state.subagentRunsBySessionId["session-1"]["job-1"].status).toBe(
      "completed"
    )
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
