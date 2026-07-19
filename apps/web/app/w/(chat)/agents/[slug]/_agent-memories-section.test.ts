import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import {
  AgentMemoriesView,
  MemoryActionsMenuContent,
  relativeTime,
  toAgentMemory,
} from "./_agent-memories-section"

const noopEdit = async () => {}
const noopForget = async () => {}

describe("AgentMemoriesView", () => {
  it("renders agent memories using the established content and tag cards", () => {
    const html = renderToString(
      React.createElement(AgentMemoriesView, {
        memories: [
          {
            id: "memory-1",
            content: "Deployments use the production Railway environment.",
            tags: ["deployment", "railway"],
            createdAt: "2026-07-18T10:00:00Z",
          },
        ],
        onEdit: noopEdit,
        onForget: noopForget,
      })
    )

    expect(html).toContain("Learned memories")
    expect(html).toContain(
      "Deployments use the production Railway environment."
    )
    expect(html).toContain("deployment")
    expect(html).toContain("railway")
    expect(html).toContain("memory</span>")
    expect(html).toContain("Memory options")
  })

  it("teaches the empty state", () => {
    const html = renderToString(
      React.createElement(AgentMemoriesView, {
        memories: [],
        onEdit: noopEdit,
        onForget: noopForget,
      })
    )

    expect(html).toContain("No memories yet")
    expect(html).toContain("as this agent learns durable facts")
  })

  it("offers edit and forget actions from the memory menu", () => {
    const html = renderToString(
      React.createElement(MemoryActionsMenuContent, {
        onEdit: () => {},
        onForget: () => {},
      })
    )

    expect(html).toContain("Edit")
    expect(html).toContain("Forget")
  })
})

describe("agent memory normalization", () => {
  it("normalizes generated API responses without response casts", () => {
    expect(
      toAgentMemory({
        id: "memory-1",
        content: "  Keep this fact.  ",
        tags: ["important", "important", "  operations  ", ""],
        created_at: "2026-07-18T10:00:00Z",
      })
    ).toEqual({
      id: "memory-1",
      content: "Keep this fact.",
      tags: ["important", "operations"],
      createdAt: "2026-07-18T10:00:00Z",
    })
  })

  it("formats relative timestamps", () => {
    const now = new Date("2026-07-19T10:00:00Z").getTime()
    expect(relativeTime("2026-07-19T09:55:00Z", now)).toBe("5m ago")
    expect(relativeTime("2026-07-18T10:00:00Z", now)).toBe("1d ago")
  })
})
