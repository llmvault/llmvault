import { describe, expect, it } from "vitest"
import {
  sessionHistoryPagesToEvents,
  sessionEventsToConversationBlocks,
  type SessionEventResponse,
} from "@/app/w/(chat)/_lib/session-history"

let sequence = 0

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

describe("sessionEventsToConversationBlocks", () => {
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

  it("does not render plan updates in the transcript", () => {
    const blocks = sessionEventsToConversationBlocks([
      event("plan_updated", {
        plan: [{ status: "in_progress", step: "1. Inspect project" }],
      }),
    ])

    expect(blocks).toEqual([])
  })

  it("treats command-bearing tool results as shell commands", () => {
    const blocks = sessionEventsToConversationBlocks([
      event("tool_result", {
        id: "tool-1",
        tool: "exec_command",
        result: {
          command: "pnpm -C apps/web typecheck",
          output: "ok",
          exit_code: 0,
        },
      }),
    ])

    expect(blocks).toHaveLength(1)
    expect(blocks[0]).toMatchObject({
      type: "worklog",
      blocks: [
        {
          type: "tool",
          label: "Ran pnpm -C apps/web typecheck",
          detail: {
            kind: "Shell",
            command: "pnpm -C apps/web typecheck",
          },
        },
      ],
    })
  })

  it("renders web search and fetch tools with parameter-aware labels", () => {
    const blocks = sessionEventsToConversationBlocks([
      event("tool_result", {
        id: "tool-web-search",
        tool: "hivy web search",
        args_summary: JSON.stringify({
          query: "test search query hivy agent tool",
        }),
        result_summary: JSON.stringify({ ok: true }),
      }),
      event("tool_result", {
        id: "tool-web-fetch",
        tool: "hivy_web_fetch",
        args_summary: JSON.stringify({
          url: "https://usehivy.com/docs",
        }),
        result_summary: JSON.stringify({ ok: true }),
      }),
    ])

    expect(blocks[0]).toMatchObject({
      type: "worklog",
      blocks: [
        {
          type: "tool",
          label: "Searched test search query hivy agent tool",
          detail: {
            category: "web_search",
            icon: "logos:chrome",
            query: "test search query hivy agent tool",
          },
        },
        {
          type: "tool",
          label: "Fetched https://usehivy.com/docs",
          detail: {
            category: "web_fetch",
            icon: "logos:chrome",
            url: "https://usehivy.com/docs",
          },
        },
      ],
    })
  })

  it("extracts web search result urls from JSON output", () => {
    const blocks = sessionEventsToConversationBlocks([
      event("tool_result", {
        id: "tool-web-search-results",
        tool: "hivy_web_search",
        args_summary: JSON.stringify({
          query: "junior.so company",
        }),
        result_summary: JSON.stringify({
          content: [
            {
              type: "text",
              text: JSON.stringify([
                {
                  url: "https://junior.so/",
                  title: "Junior - The AI Employees for Any Role",
                },
                {
                  url: "https://junior.so/press",
                  title: "Media & Press - Junior",
                },
              ]),
            },
          ],
        }),
      }),
    ])

    expect(blocks[0]).toMatchObject({
      type: "worklog",
      blocks: [
        {
          type: "tool",
          label: "Searched junior.so company",
          detail: {
            category: "web_search",
            searchResults: [
              {
                url: "https://junior.so/",
                title: "Junior - The AI Employees for Any Role",
              },
              {
                url: "https://junior.so/press",
                title: "Media & Press - Junior",
              },
            ],
          },
        },
      ],
    })
  })

  it("extracts persisted web search result urls from array summaries", () => {
    const blocks = sessionEventsToConversationBlocks([
      event("tool_result", {
        id: "tool-web-search-array-results",
        tool: "hivy_web_search",
        args_summary: JSON.stringify({
          query: "Lindy AI funding valuation employees 2026",
        }),
        result_summary: JSON.stringify([
          {
            url: "https://www.lindy.ai/",
            title: "Lindy AI",
          },
          {
            url: "https://www.lindy.ai/blog/ai-workforce",
            title: "AI Workforce",
          },
        ]),
      }),
    ])

    expect(blocks[0]).toMatchObject({
      type: "worklog",
      blocks: [
        {
          type: "tool",
          label: "Searched Lindy AI funding valuation employees 2026",
          detail: {
            category: "web_search",
            icon: "logos:chrome",
            searchResults: [
              {
                url: "https://www.lindy.ai/",
                title: "Lindy AI",
              },
              {
                url: "https://www.lindy.ai/blog/ai-workforce",
                title: "AI Workforce",
              },
            ],
          },
        },
      ],
    })
  })

  it("extracts web search result urls from truncated persisted summaries", () => {
    const blocks = sessionEventsToConversationBlocks([
      event("tool_result", {
        id: "tool-web-search-truncated-results",
        tool: "hivy_web_search",
        args_summary: JSON.stringify({
          query: "Lindy AI company overview",
        }),
        result_summary:
          '[{"url":"https://www.lindy.ai/","title":"Lindy AI"},{"url":"https://www.lindy.ai/blog/ai-workforce","title":"AI Workforce"},{"url":"https://aitoolscoop.com/ai-agents/lindy-ai/","title":"Lindy AI review"...',
      }),
    ])

    expect(blocks[0]).toMatchObject({
      type: "worklog",
      blocks: [
        {
          type: "tool",
          label: "Searched Lindy AI company overview",
          detail: {
            category: "web_search",
            icon: "logos:chrome",
            searchResults: [
              {
                url: "https://www.lindy.ai/",
              },
              {
                url: "https://www.lindy.ai/blog/ai-workforce",
              },
              {
                url: "https://aitoolscoop.com/ai-agents/lindy-ai/",
              },
            ],
          },
        },
      ],
    })
  })

  it("recognizes live web search payloads that use display tool fields", () => {
    const blocks = sessionEventsToConversationBlocks(
      [
        event("tool_call", {
          id: "tool-live-web-search",
          tool_name: "Web search",
          args: { query: "hivy agent ai tools" },
          turn_id: "turn-live-web",
        }),
      ],
      { mode: "live" }
    )

    expect(blocks[0]).toMatchObject({
      type: "worklog",
      blocks: [
        {
          type: "tool",
          label: "Searching hivy agent ai tools",
          running: true,
          detail: {
            category: "web_search",
            icon: "logos:chrome",
            query: "hivy agent ai tools",
          },
        },
      ],
    })
  })

  it("keeps web search styling after the completed result merges into a live call", () => {
    const blocks = sessionEventsToConversationBlocks(
      [
        event("tool_call", {
          id: "tool-live-web-search",
          tool_name: "Web search",
          args: { query: "junior.so company" },
          turn_id: "turn-live-web",
        }),
        event("tool_result", {
          id: "tool-live-web-search",
          result: {
            content: [
              {
                type: "text",
                text: JSON.stringify([
                  {
                    url: "https://tracxn.com/d/companies/coworkerai",
                    title: "Coworker Ai - 2026 Company Profile",
                  },
                  {
                    url: "https://sourceforge.net/software/product/Coworker.ai/alternatives",
                    title: "Best Coworker.ai Alternatives",
                  },
                ]),
              },
            ],
          },
          turn_id: "turn-live-web",
        }),
      ],
      { mode: "live" }
    )

    expect(blocks[0]).toMatchObject({
      type: "worklog",
      blocks: [
        {
          type: "tool",
          label: "Searched junior.so company",
          running: false,
          detail: {
            category: "web_search",
            icon: "logos:chrome",
            kind: "Web search",
            query: "junior.so company",
            searchResults: [
              {
                url: "https://tracxn.com/d/companies/coworkerai",
                title: "Coworker Ai - 2026 Company Profile",
              },
              {
                url: "https://sourceforge.net/software/product/Coworker.ai/alternatives",
                title: "Best Coworker.ai Alternatives",
              },
            ],
          },
        },
      ],
    })
  })

  it("hides orphan generic tool results while an active turn continues", () => {
    const blocks = sessionEventsToConversationBlocks(
      [
        event("tool_result", {
          id: "generic-orphan",
          result: { content: [{ text: "{}", type: "text" }] },
          turn_id: "turn-live-web",
        }),
        event("tool_call", {
          id: "tool-live-web-search",
          tool_name: "Web search",
          args: { query: 'Artisan AI employee "artisan" slack coworker 2026' },
          turn_id: "turn-live-web",
        }),
      ],
      { mode: "live" }
    )

    expect(blocks[0]).toMatchObject({
      type: "worklog",
      blocks: [
        {
          type: "tool",
          label: 'Searching Artisan AI employee "artisan" slack coworker 2026',
          detail: {
            category: "web_search",
            icon: "logos:chrome",
          },
        },
      ],
    })
  })

  it("classifies failed generic web fetch results from error output", () => {
    const blocks = sessionEventsToConversationBlocks([
      event("tool_result", {
        id: "tool-failed-web-fetch",
        result: {
          content: [
            {
              type: "text",
              text: "Error: web fetch failed: reading response body: context deadline exceeded",
            },
          ],
        },
        turn_id: "turn-failed-fetch",
      }),
    ])

    expect(blocks[0]).toMatchObject({
      type: "worklog",
      blocks: [
        {
          type: "tool",
          label: "Failed fetching a webpage",
          running: false,
          detail: {
            category: "web_fetch",
            icon: "logos:chrome",
            kind: "Web fetch",
            status: "errored",
            error:
              "Error: web fetch failed: reading response body: context deadline exceeded",
          },
        },
      ],
    })
  })

  it("unwraps persisted agent_event web fetch payloads", () => {
    const blocks = sessionEventsToConversationBlocks([
      event("tool_result", {
        agent_event: {
          id: "tool-agent-event-fetch",
          kind: "tool_call",
          tool: "hivy_web_fetch",
          args: { url: "https://junior.so/" },
          result: { content: [{ text: "ok", type: "text" }] },
        },
        turn_id: "turn-agent-event",
      }),
    ])

    expect(blocks[0]).toMatchObject({
      type: "worklog",
      blocks: [
        {
          type: "tool",
          label: "Fetched https://junior.so/",
          detail: {
            category: "web_fetch",
            icon: "logos:chrome",
            url: "https://junior.so/",
          },
        },
      ],
    })
  })

  it("renders read, edit, write, and skill tools with focused labels", () => {
    const diff = [
      "--- a/handler.rs",
      "+++ b/handler.rs",
      "@@ -1,1 +1,1 @@",
      "-old",
      "+new",
      "",
    ].join("\n")

    const blocks = sessionEventsToConversationBlocks([
      event("tool_result", {
        id: "tool-read",
        tool: "read_file",
        args_summary: JSON.stringify({ path: "src/handler.rs" }),
        result_summary: JSON.stringify({
          path: "/workspace/src/handler.rs",
          content: "hidden in UI",
        }),
      }),
      event("tool_result", {
        id: "tool-edit",
        tool: "edit_file",
        args_summary: JSON.stringify({ path: "src/handler.rs" }),
        result_summary: JSON.stringify({
          path: "/workspace/src/handler.rs",
          edits_applied: 1,
          diff,
        }),
      }),
      event("tool_result", {
        id: "tool-write",
        tool: "write_file",
        args_summary: JSON.stringify({ path: "src/new-file.ts" }),
        result_summary: JSON.stringify({
          path: "/workspace/src/new-file.ts",
          bytes_written: 42,
        }),
      }),
      event("tool_result", {
        id: "tool-skills",
        tool: "skill_list",
        result_summary: JSON.stringify({ skills: [] }),
      }),
    ])

    expect(blocks[0]).toMatchObject({
      type: "worklog",
      blocks: [
        {
          type: "tool",
          label: "Read handler.rs",
          detail: {
            category: "file_read",
            icon: "lucide:file-text",
            paths: ["src/handler.rs", "/workspace/src/handler.rs"],
          },
        },
        {
          type: "tool",
          label: "Edited a file",
          detail: {
            category: "file_edit",
            icon: "lucide:pencil",
            diff,
            editsApplied: 1,
          },
        },
        {
          type: "tool",
          label: "Wrote a file",
          detail: {
            category: "file_write",
            icon: "lucide:file-plus",
            bytesWritten: 42,
          },
        },
        {
          type: "tool",
          label: "Listed skills",
          detail: {
            category: "skill_list",
            icon: "lucide:sparkles",
          },
        },
      ],
    })
  })

  it("uses specific memory verbs for retain and recall tools", () => {
    const blocks = sessionEventsToConversationBlocks(
      [
        event("tool_call", {
          id: "tool-retain",
          tool: "hivy_memory_retain",
          args: { text: "customer prefers GitHub" },
          turn_id: "turn-memory",
        }),
        event("tool_result", {
          id: "tool-recall",
          tool: "hivy_memory_recall",
          args_summary: JSON.stringify({ query: "customer preferences" }),
          result_summary: JSON.stringify({ memories: [] }),
          turn_id: "turn-memory",
        }),
      ],
      { mode: "live" }
    )

    expect(blocks[0]).toMatchObject({
      type: "worklog",
      blocks: [
        {
          type: "tool",
          label: "Retaining memory: customer prefers GitHub",
          detail: {
            category: "memory",
            icon: "lucide:brain",
            tool: "hivy_memory_retain",
          },
        },
        {
          type: "tool",
          label: "Recalled memory: customer preferences",
          detail: {
            category: "memory",
            icon: "lucide:brain",
            tool: "hivy_memory_recall",
          },
        },
      ],
    })
  })

  it("renders session search with session-specific labels and icon", () => {
    const blocks = sessionEventsToConversationBlocks(
      [
        event("tool_call", {
          id: "tool-search-sessions",
          tool: "Search_sessions",
          args: { query: "test" },
          turn_id: "turn-session-search",
        }),
        event("tool_result", {
          id: "tool-search-sessions",
          tool: "Search_sessions",
          args_summary: JSON.stringify({ query: "test" }),
          result_summary: JSON.stringify({ ok: true }),
          turn_id: "turn-session-search",
        }),
      ],
      { mode: "live" }
    )

    expect(blocks[0]).toMatchObject({
      type: "worklog",
      blocks: [
        {
          type: "tool",
          label: "Searched sessions: test",
          detail: {
            category: "session_search",
            icon: "lucide:messages-square",
            query: "test",
          },
        },
      ],
    })
  })

  it("marks only the trailing live thinking block as active", () => {
    const blocks = sessionEventsToConversationBlocks(
      [
        event("thinking", { text: "first thought", turn_id: "turn-1" }),
        event("tool_result", {
          id: "tool-1",
          tool: "bash",
          turn_id: "turn-1",
          result: { command: "pwd", output: "/workspace" },
        }),
        event("thinking", { text: "second thought", turn_id: "turn-1" }),
      ],
      { mode: "live" }
    )

    expect(blocks).toHaveLength(1)
    expect(blocks[0]).toMatchObject({
      type: "worklog",
      active: true,
      defaultExpanded: true,
    })
    const worklog = blocks[0]
    const thoughts =
      worklog.type === "worklog"
        ? worklog.blocks.filter((block) => block.type === "thinking")
        : []
    expect(thoughts).toMatchObject([
      { label: "Thought", active: false },
      { label: "Thinking", active: true },
    ])
  })

  it("renders pending optimistic thinking without a worklog before tools", () => {
    const blocks = sessionEventsToConversationBlocks([
      event("thinking", {
        text: "",
        turn_id: "turn-pending",
        client_status: "pending",
      }),
    ])

    expect(blocks).toMatchObject([
      {
        type: "thinking",
        label: "Thinking",
        active: true,
        defaultExpanded: true,
      },
    ])
  })

  it("does not leave live thinking active after the stream moves to a tool", () => {
    const blocks = sessionEventsToConversationBlocks(
      [
        event("thinking", { text: "checking", turn_id: "turn-2" }),
        event("tool_call", {
          id: "tool-2",
          tool: "bash",
          turn_id: "turn-2",
          args: { command: "make up" },
        }),
      ],
      { mode: "live" }
    )

    const worklog = blocks[0]
    const thoughts =
      worklog.type === "worklog"
        ? worklog.blocks.filter((block) => block.type === "thinking")
        : []
    expect(thoughts).toMatchObject([{ label: "Thought", active: false }])
  })

  it("renders live assistant tokens directly before tools", () => {
    const blocks = sessionEventsToConversationBlocks(
      [event("token", { text: "Still working", turn_id: "turn-3" })],
      { mode: "live" }
    )

    expect(blocks).toEqual([
      {
        type: "assistant",
        key: expect.any(String),
        text: "Still working",
        streaming: true,
      },
    ])
  })

  it("renders standalone thought, not a worklog, for completed turns without tool calls", () => {
    const blocks = sessionEventsToConversationBlocks([
      event("turn_started", { turn_id: "turn-no-tools" }),
      event("thinking", {
        text: "I can answer this directly.",
        turn_id: "turn-no-tools",
      }),
      event("token", {
        text: "hello",
        turn_id: "turn-no-tools",
      }),
      event("final", {
        text: "hello",
        turn_id: "turn-no-tools",
      }),
      event("turn_completed", { turn_id: "turn-no-tools" }),
    ])

    expect(blocks).toEqual([
      {
        type: "thinking",
        key: expect.any(String),
        label: "Thought",
        duration: undefined,
        text: "I can answer this directly.",
      },
      {
        type: "assistant",
        key: expect.any(String),
        text: "hello",
        streaming: false,
      },
      { type: "actions", key: expect.any(String) },
    ])
  })

  it("keeps completed-message actions for persisted assistant history", () => {
    const blocks = sessionEventsToConversationBlocks([
      event("final", { text: "Done", turn_id: "turn-4" }),
    ])

    expect(blocks).toEqual([
      {
        type: "assistant",
        text: "Done",
        key: expect.any(String),
        streaming: false,
      },
      { type: "actions", key: expect.any(String) },
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

  it("collapses completed turn work before the final answer", () => {
    const blocks = sessionEventsToConversationBlocks([
      event("turn_started", { turn_id: "turn-5" }),
      event("thinking", { text: "checking", turn_id: "turn-5" }),
      event("tool_result", {
        id: "tool-5",
        tool: "bash",
        result: { command: "pwd", output: "/workspace" },
        turn_id: "turn-5",
      }),
      event("token", {
        text: "I found the repo.",
        turn_id: "turn-5",
      }),
      event("token", {
        text: "The repo is ready.",
        turn_id: "turn-5",
      }),
      event("final", {
        text: "The repo is ready.",
        turn_id: "turn-5",
      }),
      event("turn_completed", { turn_id: "turn-5" }),
    ])

    expect(blocks).toMatchObject([
      {
        type: "worklog",
        active: false,
        defaultExpanded: false,
        blocks: [
          { type: "thinking" },
          { type: "tool" },
          { type: "assistant", text: "I found the repo." },
        ],
      },
      { type: "assistant", text: "The repo is ready." },
      { type: "actions" },
    ])
  })
})
