import type { components } from "@/lib/api/schema"

export type SessionEventResponse = components["schemas"]["sessionEventResponse"]

export type SessionPlanStepStatus = "completed" | "in_progress" | "pending"

export interface SessionPlanStep {
  status: SessionPlanStepStatus
  text: string
}

export interface SessionPlan {
  key: string
  steps: SessionPlanStep[]
  activeIndex: number
  updatedAt: number
}

type Payload = Record<string, unknown>

export function latestSessionPlan(
  events: SessionEventResponse[]
): SessionPlan | null {
  let latest: SessionPlan | null = null

  for (const event of events) {
    const plan = sessionPlanFromEvent(event)
    if (!plan) continue
    if (!latest || plan.updatedAt >= latest.updatedAt) {
      latest = plan
    }
  }

  return latest
}

export function sessionPlanFromEvent(
  event: SessionEventResponse
): SessionPlan | null {
  const payload = payloadRecord(event)
  const eventName = stringValue(payload, "event_name")
  if (event.event_type !== "plan_updated" && eventName !== "plan_updated") {
    return null
  }

  const rawPlan = payload.plan
  if (!Array.isArray(rawPlan)) return null

  const steps = rawPlan.flatMap((entry): SessionPlanStep[] => {
    if (!entry || typeof entry !== "object" || Array.isArray(entry)) {
      return []
    }
    const record = entry as Payload
    const text = cleanStepText(stringValue(record, "step"))
    if (!text) return []
    return [
      {
        status: normalizeStatus(stringValue(record, "status")),
        text,
      },
    ]
  })

  if (!steps.length) return null

  return {
    key: event.id ?? event.event_id ?? `plan:${event.sequence_number ?? 0}`,
    steps,
    activeIndex: activeStepIndex(steps),
    updatedAt: eventTime(event),
  }
}

function activeStepIndex(steps: SessionPlanStep[]) {
  const inProgress = steps.findIndex((step) => step.status === "in_progress")
  if (inProgress >= 0) return inProgress
  const pending = steps.findIndex((step) => step.status === "pending")
  if (pending >= 0) return pending
  return steps.length - 1
}

function normalizeStatus(status: string): SessionPlanStepStatus {
  const normalized = status
    .trim()
    .toLowerCase()
    .replace(/[\s-]+/g, "_")
  if (normalized === "completed" || normalized === "complete") {
    return "completed"
  }
  if (
    normalized === "in_progress" ||
    normalized === "inprogress" ||
    normalized === "active" ||
    normalized === "running" ||
    normalized === "working"
  ) {
    return "in_progress"
  }
  return "pending"
}

function cleanStepText(step: string) {
  return step.replace(/^\s*\d+[\).]\s*/, "").trim()
}

function eventTime(event: SessionEventResponse): number {
  const payload = payloadRecord(event)
  const occurredAt = stringValue(payload, "occurred_at")
  const time = Date.parse(occurredAt || event.event_at || "")
  if (!Number.isNaN(time)) return time
  return event.sequence_number ?? 0
}

function payloadRecord(event: SessionEventResponse): Payload {
  const payload = event.payload
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    return {}
  }
  return payload as Payload
}

function stringValue(payload: Payload, key: string): string {
  const value = payload[key]
  return typeof value === "string" ? value : ""
}
