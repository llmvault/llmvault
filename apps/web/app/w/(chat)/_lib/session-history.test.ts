import { beforeEach, describe, expect, it } from "vitest"
import {
  sessionEventsToConversationBlocks,
  sessionHistoryPagesToEvents,
  type SessionEventResponse,
} from "@/app/w/(chat)/_lib/session-history"

let sequence = 0

beforeEach(() => {
  sequence = 0
})

function event(
  eventType: string,
  payload: Record<string, unknown>,
  eventAt = "2026-06-15T12:00:00.000Z"
): SessionEventResponse {
  sequence += 1
  return {
    id: `event-${sequence}`,
    event_id: `event-${sequence}`,
    event_type: eventType,
    sequence_number: sequence,
    payload,
    event_at: eventAt,
  } as SessionEventResponse
}

function tool(
  id: string,
  command: string,
  turnID = "turn-tools"
): SessionEventResponse {
  return event("tool_result", {
    id,
    tool: "bash",
    result: { command, output: "ok", exit_code: 0 },
    turn_id: turnID,
  })
}

describe("sessionHistoryPagesToEvents", () => {
  it("merges paginated history without duplicate boundary events", () => {
    const first = event("token", { text: "newest", turn_id: "turn-page" })
    const duplicate = event("thinking", {
      text: "shared",
      turn_id: "turn-page",
    })
    const oldest = event("token", { text: "oldest", turn_id: "turn-page" })

    const events = sessionHistoryPagesToEvents([
      { data: [first, duplicate] },
      { data: [duplicate, oldest] },
    ])

    expect(events).toEqual([first, duplicate, oldest])
  })

  it("dedupes live and persisted copies of the same runtime event", () => {
    const persisted = {
      ...event("thinking", {
        text: "shared",
        turn_id: "turn-page",
      }),
      id: "db-event-id",
      event_id: "runtime-event-id",
    }
    const live = {
      ...persisted,
      id: "runtime-event-id",
    }

    const events = sessionHistoryPagesToEvents([
      { data: [persisted] },
      { data: [live] },
    ])

    expect(events).toEqual([persisted])
  })
})

describe("sessionEventsToConversationBlocks", () => {
  it("keeps visible block keys stable when older history is prepended", () => {
    const current = event("final", {
      text: "Visible answer",
      turn_id: "turn-visible",
    })
    const older = event("final", {
      text: "Older answer",
      turn_id: "turn-older",
    })

    const currentBlocks = sessionEventsToConversationBlocks([current])
    const prependedBlocks = sessionEventsToConversationBlocks([older, current])

    const currentAnswer = currentBlocks.find(
      (block) => block.type === "assistant" && block.text === "Visible answer"
    )
    const prependedAnswer = prependedBlocks.find(
      (block) => block.type === "assistant" && block.text === "Visible answer"
    )

    expect(prependedAnswer?.key).toBe(currentAnswer?.key)
  })

  it("keeps final assistant completion time for the message footer", () => {
    const completedAt = "2026-06-15T16:11:00.000Z"
    const blocks = sessionEventsToConversationBlocks([
      event("final", { text: "Done." }, completedAt),
    ])

    expect(blocks).toMatchObject([
      {
        type: "assistant",
        text: "Done.",
        completedAt,
      },
    ])
  })

  it("does not render plan updates in the transcript", () => {
    const blocks = sessionEventsToConversationBlocks([
      event("plan_updated", {
        plan: [{ status: "in_progress", step: "1. Inspect project" }],
      }),
    ])

    expect(blocks).toEqual([])
  })

  it("does not render request-user-input lifecycle events in the transcript", () => {
    const blocks = sessionEventsToConversationBlocks([
      event("question_requested", {
        question_request_id: "question-request-1",
        questions: [
          {
            id: "deployment_path",
            header: "Deploy",
            question: "Which deployment path should Codex use?",
            options: [
              { label: "Ship It", description: "Deploy immediately." },
              { label: "Hold", description: "Wait for review." },
            ],
          },
        ],
      }),
      event("question_answered", {
        question_request_id: "question-request-1",
        answers: { deployment_path: { answers: ["Ship It"] } },
      }),
    ])

    expect(blocks).toEqual([])
  })

  it("renders user attachments and removes attachment XML from text", () => {
    const blocks = sessionEventsToConversationBlocks([
      event("user.message", {
        text: `Please use this screenshot

<attachment name="screen.png" url="https://api.test/v1/assets/preview?path=x" mime_type="image/png">
<description>
Primary category: Product UI
</description>
</attachment>`,
        attachments: [
          {
            drive_asset_id: "asset-1",
            asset_url: "https://api.test/v1/assets/preview?path=x",
            filename: "screen.png",
            content_type: "image/png",
          },
        ],
      }),
    ])

    expect(blocks).toMatchObject([
      {
        type: "user",
        text: "Please use this screenshot",
        attachments: [
          {
            id: "asset-1",
            filename: "screen.png",
            kind: "image",
            url: "https://api.test/v1/assets/preview?path=x",
          },
        ],
      },
    ])
  })

  it("renders code comments from user message metadata without requiring text", () => {
    const blocks = sessionEventsToConversationBlocks([
      event("user.message", {
        text: "",
        code_line_comments: [
          {
            id: "comment-1",
            source_key: "repo:apps/web/lib/diffs-theme.ts",
            source_kind: "review",
            path: "apps/web/lib/diffs-theme.ts",
            display_path: "apps/web/lib/diffs-theme.ts",
            line_number: 148,
            side: "additions",
            body: "Use the HeroUI token here.",
            created_at: 1781900000000,
          },
        ],
      }),
    ])

    expect(blocks).toMatchObject([
      {
        type: "user",
        text: "",
        codeLineComments: [
          {
            id: "comment-1",
            displayPath: "apps/web/lib/diffs-theme.ts",
            lineNumber: 148,
            side: "additions",
            body: "Use the HeroUI token here.",
          },
        ],
      },
    ])
  })

  it("groups pre-final agent activity into a worked block", () => {
    const blocks = sessionEventsToConversationBlocks([
      event(
        "turn_started",
        { turn_id: "turn-flat" },
        "2026-06-15T12:00:00.000Z"
      ),
      event("token", {
        text: "I will inspect this first.",
        turn_id: "turn-flat",
      }),
      tool("tool-1", "pwd", "turn-flat"),
      tool("tool-2", "ls", "turn-flat"),
      tool("tool-3", "rg session", "turn-flat"),
      event("thinking", {
        text: "The session UI owns the complexity.",
        turn_id: "turn-flat",
      }),
      tool("tool-4", "sed -n '1,120p' session-history.ts", "turn-flat"),
      tool("tool-5", "pnpm test session-history", "turn-flat"),
      event("final", {
        text: "The transcript is now flat.",
        turn_id: "turn-flat",
      }),
      event(
        "turn_completed",
        { turn_id: "turn-flat" },
        "2026-06-15T12:04:43.000Z"
      ),
    ])

    expect(blocks).toMatchObject([
      {
        type: "agent_work",
        duration: "4m 43s",
        active: false,
        defaultExpanded: false,
        blocks: [
          { type: "assistant", text: "I will inspect this first." },
          {
            type: "tool_chain",
            tools: [
              { type: "tool", label: "Ran pwd" },
              { type: "tool", label: "Ran ls" },
              { type: "tool", label: "Ran rg session" },
            ],
          },
          { type: "thinking", text: "The session UI owns the complexity." },
          { type: "tool", label: "Ran sed -n '1,120p' session-history.ts" },
          { type: "tool", label: "Ran pnpm test session-history" },
        ],
      },
      { type: "assistant", text: "The transcript is now flat." },
    ])
  })

  it("only groups adjacent tool runs when there are more than two calls", () => {
    const twoTools = sessionEventsToConversationBlocks([
      tool("tool-1", "pwd"),
      tool("tool-2", "ls"),
    ])
    const threeTools = sessionEventsToConversationBlocks([
      tool("tool-1", "pwd"),
      tool("tool-2", "ls"),
      tool("tool-3", "rg session"),
    ])

    expect(twoTools).toMatchObject([
      {
        type: "agent_work",
        blocks: [{ type: "tool" }, { type: "tool" }],
      },
    ])
    expect(threeTools).toMatchObject([
      {
        type: "agent_work",
        blocks: [
          {
            type: "tool_chain",
            tools: [
              { label: "Ran pwd" },
              { label: "Ran ls" },
              { label: "Ran rg session" },
            ],
          },
        ],
      },
    ])
  })

  it("merges a tool call and result before deciding whether to group", () => {
    const blocks = sessionEventsToConversationBlocks([
      event("tool_call", {
        id: "tool-1",
        tool: "bash",
        args: { command: "ls" },
        turn_id: "turn-tools",
      }),
      event("tool_result", {
        id: "tool-1",
        tool: "bash",
        result: { command: "ls", output: "README.md", exit_code: 0 },
        turn_id: "turn-tools",
      }),
    ])

    expect(blocks).toMatchObject([
      {
        type: "agent_work",
        blocks: [
          {
            type: "tool",
            label: "Ran ls",
            running: false,
            detail: {
              command: "ls",
              output: "README.md",
            },
          },
        ],
      },
    ])
  })

  it("uses browser command metadata for bash browser calls", () => {
    const blocks = sessionEventsToConversationBlocks([
      event("tool_result", {
        id: "tool-browser",
        tool: "bash",
        result: {
          command: "if ready; then browser screenshot --full /tmp/page.png; fi",
          output: "/tmp/page.png",
          exit_code: 0,
        },
        turn_id: "turn-browser-tool",
      }),
    ])

    expect(blocks).toMatchObject([
      {
        type: "agent_work",
        blocks: [
          {
            type: "tool",
            label: "Took a screenshot",
            detail: {
              kind: "Chrome browser",
              icon: "logos:chrome",
              actionIcon: "lucide:camera",
            },
          },
        ],
      },
    ])
  })

  it("skips the token immediately duplicated by a final answer", () => {
    const blocks = sessionEventsToConversationBlocks([
      event("thinking", {
        text: "Checking",
        turn_id: "turn-repeated-token",
      }),
      tool("tool-1", "true", "turn-repeated-token"),
      event("token", {
        text: "Done. ",
        turn_id: "turn-repeated-token",
      }),
      event("token", {
        text: "More context.",
        turn_id: "turn-repeated-token",
      }),
      event("token", {
        text: "Done.",
        turn_id: "turn-repeated-token",
      }),
      event("final", {
        text: "Done.",
        turn_id: "turn-repeated-token",
      }),
    ])

    expect(blocks).toMatchObject([
      {
        type: "agent_work",
        blocks: [
          { type: "thinking", text: "Checking" },
          { type: "tool", label: "Ran true" },
          { type: "assistant", text: "Done. More context." },
        ],
      },
      { type: "assistant", text: "Done." },
    ])
  })

  it("preserves thinking chunk boundary whitespace", () => {
    const blocks = sessionEventsToConversationBlocks([
      event("thinking", {
        text: "Let",
        turn_id: "turn-thinking",
      }),
      event("thinking", {
        text: " me",
        turn_id: "turn-thinking",
      }),
      event("thinking", {
        text: " now",
        turn_id: "turn-thinking",
      }),
    ])

    expect(blocks).toMatchObject([
      {
        type: "agent_work",
        blocks: [{ type: "thinking", text: "Let me now" }],
      },
    ])
  })

  it("keeps active pre-final work open while the turn is running", () => {
    const blocks = sessionEventsToConversationBlocks(
      [event("token", { text: "Still working", turn_id: "turn-live" })],
      { activeTurnID: "turn-live" }
    )

    expect(blocks).toMatchObject([
      {
        type: "agent_work",
        key: expect.any(String),
        active: true,
        defaultExpanded: true,
        blocks: [
          {
            type: "assistant",
            text: "Still working",
            streaming: true,
          },
        ],
      },
    ])
  })

  it("uses custom labels for active thinking events", () => {
    const thinking = event("thinking", {
      label: "Thinking...",
      turn_id: "turn-model",
    })

    const blocks = sessionEventsToConversationBlocks([thinking], {
      activeTurnID: "turn-model",
    })

    expect(blocks).toMatchObject([
      {
        type: "agent_work",
        active: true,
        blocks: [
          {
            type: "thinking",
            label: "Thinking...",
            active: true,
          },
        ],
      },
    ])
  })

  it("keeps active work after the user message that started it", () => {
    const blocks = sessionEventsToConversationBlocks(
      [
        event(
          "thinking",
          {
            text: "",
            turn_id: "turn-live",
          },
          "2026-06-15T12:00:00.000Z"
        ),
        event(
          "user.message",
          { text: "what about this one?" },
          "2026-06-15T12:00:05.000Z"
        ),
      ],
      { activeTurnID: "turn-live" }
    )

    expect(blocks).toMatchObject([
      { type: "user", text: "what about this one?" },
      {
        type: "agent_work",
        active: true,
        blocks: [{ type: "thinking", label: "Thinking" }],
      },
    ])
  })

  it("renders turn errors as the terminal section after work", () => {
    const blocks = sessionEventsToConversationBlocks([
      event("thinking", {
        text: "Checking",
        turn_id: "turn-error-after-work",
      }),
      tool("tool-1", "false", "turn-error-after-work"),
      event("error", {
        message: "The agent turn failed.",
        turn_id: "turn-error-after-work",
      }),
    ])

    expect(blocks).toMatchObject([
      {
        type: "agent_work",
        active: false,
        blocks: [
          { type: "thinking", text: "Checking" },
          { type: "tool", label: "Ran false" },
        ],
      },
      {
        type: "error",
        text: "The agent turn failed.",
      },
    ])
  })

  it("renders stream error frames as visible error blocks", () => {
    const blocks = sessionEventsToConversationBlocks([
      event("error", {
        message: "The live session stream failed.",
        turn_id: "turn-error",
      }),
    ])

    expect(blocks).toEqual([
      {
        type: "error",
        key: expect.any(String),
        text: "The live session stream failed.",
      },
    ])
  })
})
