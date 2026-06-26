import { beforeEach, describe, expect, it } from "vitest"
import {
  sessionEventsToConversationBlocks,
  type SessionEventResponse,
} from "@/app/w/(chat)/_lib/session-history"

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
})
