import { describe, expect, it } from "vitest"
import {
  latestSessionPlan,
  sessionPlanFromEvent,
  type SessionEventResponse,
} from "@/app/w/(chat)/_lib/session-plan"

describe("session plan events", () => {
  it("normalizes plan_updated events", () => {
    const plan = sessionPlanFromEvent(
      event("plan-1", "2026-06-19T07:04:17.000Z", [
        { status: "completed", step: "1. Fetch projects" },
        { status: "in_progress", step: "2. Inspect environments" },
        { status: "pending", step: "3. Summarize findings" },
      ])
    )

    expect(plan).toMatchObject({
      activeIndex: 1,
      steps: [
        { status: "completed", text: "Fetch projects" },
        { status: "in_progress", text: "Inspect environments" },
        { status: "pending", text: "Summarize findings" },
      ],
    })
  })

  it("returns the latest plan event", () => {
    const latest = latestSessionPlan([
      event("old", "2026-06-19T07:04:17.000Z", [
        { status: "in_progress", step: "1. Old step" },
      ]),
      event("new", "2026-06-19T07:05:17.000Z", [
        { status: "completed", step: "1. Old step" },
        { status: "in_progress", step: "2. New step" },
      ]),
    ])

    expect(latest?.key).toBe("new")
    expect(latest?.activeIndex).toBe(1)
  })

  it("accepts common in-progress status spellings", () => {
    const plan = sessionPlanFromEvent(
      event("plan-statuses", "2026-06-19T07:04:17.000Z", [
        { status: "complete", step: "1. Done" },
        { status: "in progress", step: "2. Running" },
      ])
    )

    expect(plan?.steps).toEqual([
      { status: "completed", text: "Done" },
      { status: "in_progress", text: "Running" },
    ])
  })
})

function event(
  id: string,
  eventAt: string,
  plan: { status: string; step: string }[]
): SessionEventResponse {
  return {
    id,
    event_id: id,
    event_type: "plan_updated",
    event_at: eventAt,
    payload: {
      event_name: "plan_updated",
      occurred_at: eventAt,
      plan,
    },
  } as SessionEventResponse
}
