import React from "react"
import { renderToString } from "react-dom/server"
import { describe, expect, it } from "vitest"
import {
  ModelChoiceSection,
  modelAssignmentSequence,
  updateModelAssignments,
} from "./model-choice-section"

describe("ModelChoiceSection", () => {
  it("uses one shared origin for every assigned-model column", () => {
    const html = renderToString(React.createElement(ModelChoiceSection))
    const sharedColumnTracks =
      html.match(/grid-cols-\[minmax\(0,1fr\)_minmax\(7\.5rem,42%\)\]/g) ?? []

    expect(sharedColumnTracks).toHaveLength(5)
  })
})

describe("modelAssignmentSequence", () => {
  it("assigns each round from the first agent row to the last", () => {
    expect(modelAssignmentSequence.map((step) => step.agentID)).toEqual([
      "support",
      "research",
      "code-review",
      "revenue",
      "operations",
      "support",
      "research",
      "code-review",
      "revenue",
      "operations",
    ])
  })
})

describe("updateModelAssignments", () => {
  it("preserves existing agents while replacing only the newly assigned agent", () => {
    const withSupport = updateModelAssignments(
      {},
      { agentID: "support", modelID: "nemotron-3-ultra-550b-a55b" }
    )
    const withResearch = updateModelAssignments(withSupport, {
      agentID: "research",
      modelID: "mimo-v2.5-pro",
    })
    const supportReassigned = updateModelAssignments(withResearch, {
      agentID: "support",
      modelID: "deepseek-v4-flash",
    })

    expect(withResearch).toEqual({
      support: "nemotron-3-ultra-550b-a55b",
      research: "mimo-v2.5-pro",
    })
    expect(supportReassigned).toEqual({
      support: "deepseek-v4-flash",
      research: "mimo-v2.5-pro",
    })
  })
})
