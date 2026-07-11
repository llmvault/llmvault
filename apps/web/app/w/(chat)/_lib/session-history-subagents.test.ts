import { beforeEach, describe, expect, it } from "vitest"
import {
  sessionEventsToConversationBlocks,
  type SessionEventResponse,
} from "@/app/w/(chat)/_lib/session-history"
import { appendLiveSessionStreamFrame } from "@/app/w/(chat)/_lib/live-session-stream"
import type { GoSessionStreamFrame } from "@/app/w/(chat)/_lib/go-session-stream"

let sequence = 0

beforeEach(() => {
  sequence = 0
})

function event(
  eventType: string,
  payload: Record<string, unknown>
): SessionEventResponse {
  sequence += 1
  return {
    id: `event-${sequence}`,
    event_id: `event-${sequence}`,
    event_type: eventType,
    sequence_number: sequence,
    payload,
    event_at: "2026-06-15T12:00:00.000Z",
  } as SessionEventResponse
}

function tool(id: string, command: string): SessionEventResponse {
  return event("tool_result", {
    id,
    tool: "bash",
    result: { command, output: "ok", exit_code: 0 },
    turn_id: "turn-subagents",
  })
}

describe("session subagent conversation blocks", () => {
  it("renders subagent tasks inline instead of accumulating them with other tools", () => {
    const blocks = sessionEventsToConversationBlocks(
      [
        tool("tool-1", "pwd"),
        tool("tool-2", "ls"),
        event("tool_call", {
          id: "subagent-tool-1",
          tool: "subagent_task",
          args: {
            agent: "codebase-brand-extractor",
            goal: "Extract brand rules",
          },
          turn_id: "turn-subagents",
        }),
        event("tool_result", {
          id: "subagent-tool-1",
          tool: "subagent_task",
          result: {
            job_id: "subagent-task-1",
            session_id: "subagent-subagent-task-1",
            agent: "codebase-brand-extractor",
            result: "done",
          },
          turn_id: "turn-subagents",
        }),
        tool("tool-3", "rg brand"),
        tool("tool-4", "sed -n '1,40p' README.md"),
        tool("tool-5", "pnpm test brand"),
      ],
      {
        subagentRuns: [
          {
            jobId: "subagent-task-1",
            agentName: "codebase-brand-extractor",
            childSessionId: "subagent-subagent-task-1",
            status: "completed",
            frames: [],
            events: [],
            latestText: "Brand rules extracted",
            updatedAt: 1,
          },
        ],
      }
    )

    expect(blocks).toMatchObject([
      {
        type: "agent_work",
        blocks: [
          { type: "tool", label: "Ran pwd" },
          { type: "tool", label: "Ran ls" },
          {
            type: "subagent",
            jobId: "subagent-task-1",
            agentName: "codebase-brand-extractor",
            goal: "Extract brand rules",
            childSessionId: "subagent-subagent-task-1",
            status: "completed",
            preview: "Brand rules extracted",
          },
          {
            type: "tool_chain",
            tools: [
              { label: "Ran rg brand" },
              { label: "Ran sed -n '1,40p' README.md" },
              { label: "Ran pnpm test brand" },
            ],
          },
        ],
      },
    ])
  })

  it("preserves two same-name subagent cards when live results omit the tool name", () => {
    const frames: GoSessionStreamFrame[] = [
      liveFrame("tool_call", "call-frame-1", {
        id: "subagent-call-1",
        tool: "subagent_task",
        args: {
          agent: "codebase-explorer",
          goal: "Inspect auth",
          job_id: "subagent-task-1",
          child_session_id: "subagent-session-1",
        },
        turn_id: "turn-subagents",
        sequence: 1,
      }),
      liveFrame("tool_call", "call-frame-2", {
        id: "subagent-call-2",
        tool: "subagent_task",
        args: {
          agent: "codebase-explorer",
          goal: "Inspect channels",
          job_id: "subagent-task-2",
          child_session_id: "subagent-session-2",
        },
        turn_id: "turn-subagents",
        sequence: 2,
      }),
      liveFrame("tool_result", "result-frame-1", {
        id: "subagent-call-1",
        result: {
          job_id: "subagent-task-1",
          session_id: "subagent-session-1",
          agent: "codebase-explorer",
          result: "Auth inspected",
        },
        turn_id: "turn-subagents",
        sequence: 3,
      }),
      liveFrame("tool_result", "result-frame-2", {
        id: "subagent-call-2",
        result: {
          job_id: "subagent-task-2",
          session_id: "subagent-session-2",
          agent: "codebase-explorer",
          result: "Channels inspected",
        },
        turn_id: "turn-subagents",
        sequence: 4,
      }),
    ]
    const runningEvents = frames
      .slice(0, 2)
      .reduce(appendLiveSessionStreamFrame, [])
    const runningBlocks = sessionEventsToConversationBlocks(runningEvents, {
      mode: "live",
      subagentRuns: [
        subagentRun("subagent-task-1", "subagent-session-1"),
        subagentRun("subagent-task-2", "subagent-session-2"),
      ],
    })
    const runningWork = runningBlocks.find(
      (block) => block.type === "agent_work"
    )
    expect(runningWork).toMatchObject({
      type: "agent_work",
      blocks: [
        { type: "subagent", jobId: "subagent-task-1" },
        { type: "subagent", jobId: "subagent-task-2" },
      ],
    })

    const events = frames.reduce(appendLiveSessionStreamFrame, [])
    const blocks = sessionEventsToConversationBlocks(events, {
      mode: "live",
      subagentRuns: [
        subagentRun("subagent-task-1", "subagent-session-1"),
        subagentRun("subagent-task-2", "subagent-session-2"),
      ],
    })
    const work = blocks.find((block) => block.type === "agent_work")

    expect(work).toMatchObject({
      type: "agent_work",
      blocks: [
        {
          type: "subagent",
          key: "subagent:subagent-task-1",
          jobId: "subagent-task-1",
          childSessionId: "subagent-session-1",
          agentName: "codebase-explorer",
          goal: "Inspect auth",
        },
        {
          type: "subagent",
          key: "subagent:subagent-task-2",
          jobId: "subagent-task-2",
          childSessionId: "subagent-session-2",
          agentName: "codebase-explorer",
          goal: "Inspect channels",
        },
      ],
    })
  })
})

function liveFrame(
  eventType: string,
  id: string,
  data: Record<string, unknown>
): GoSessionStreamFrame {
  return {
    sessionId: "session-1",
    event: eventType,
    id,
    data,
  }
}

function subagentRun(jobId: string, childSessionId: string) {
  return {
    jobId,
    childSessionId,
    agentName: "codebase-explorer",
    status: "completed" as const,
    frames: [],
    events: [],
    updatedAt: 1,
  }
}
