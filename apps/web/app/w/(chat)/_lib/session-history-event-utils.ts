import type { components } from "@/lib/api/schema"

export type SessionEventResponse = components["schemas"]["sessionEventResponse"]
type Payload = Record<string, unknown>

export function compareSessionEvents(
  left: SessionEventResponse,
  right: SessionEventResponse
) {
  const byTime = eventTime(left) - eventTime(right)
  if (byTime !== 0) return byTime
  return (left.sequence_number ?? 0) - (right.sequence_number ?? 0)
}

export function eventTime(event: SessionEventResponse): number {
  const time = event.event_at ? Date.parse(event.event_at) : 0
  return Number.isNaN(time) ? 0 : time
}

export function payloadRecord(event: SessionEventResponse): Payload {
  const payload = event.payload
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    return {}
  }
  return payload as Payload
}

export function stringValue(payload: Payload, key: string): string {
  const value = payload[key]
  return typeof value === "string" ? value : ""
}

export function stringRecordValue(
  record: Record<string, unknown>,
  key: string
): string {
  const value = record[key]
  return typeof value === "string" ? value : ""
}

export function eventText(event: SessionEventResponse): string {
  const payload = payloadRecord(event)
  return (
    stringValue(payload, "text") ||
    stringValue(payload, "message") ||
    stringValue(payload, "content") ||
    stringValue(payload, "markdown") ||
    stringValue(payload, "result_summary")
  )
}

export function eventTurnID(event: SessionEventResponse): string {
  return event.turn_id || stringValue(payloadRecord(event), "turn_id")
}

export function stripAttachmentTags(text: string): string {
  return text.replace(/<attachment\b[\s\S]*?<\/attachment>/gi, "").trim()
}

export function eventBlockKey(
  event: SessionEventResponse,
  prefix: string
): string {
  return `${prefix}:${event.id ?? event.event_id ?? event.sequence_number ?? event.event_at ?? "unknown"}`
}

export function eventErrorText(event: SessionEventResponse): string {
  const payload = payloadRecord(event)
  return (
    stringValue(payload, "error") ||
    stringValue(payload, "message") ||
    stringValue(payload, "text") ||
    "The session stream failed. Try sending your message again."
  )
}

export function parseTimestamp(value: string): number | undefined {
  if (!value) return undefined
  const time = Date.parse(value)
  return Number.isNaN(time) ? undefined : time
}

export function formatDuration(durationMs: number): string {
  if (durationMs < 1000) {
    return `${Math.max(0.1, Math.round(durationMs / 100) / 10)} seconds`
  }
  if (durationMs < 60_000) {
    const seconds = Math.round(durationMs / 1000)
    return seconds === 1 ? "1 second" : `${seconds} seconds`
  }
  if (durationMs < 3_600_000) {
    const totalSeconds = Math.round(durationMs / 1000)
    const minutes = Math.floor(totalSeconds / 60)
    const seconds = totalSeconds % 60
    return seconds === 0 ? `${minutes}m` : `${minutes}m ${seconds}s`
  }
  const totalMinutes = Math.round(durationMs / 60_000)
  const hours = Math.floor(totalMinutes / 60)
  const minutes = totalMinutes % 60
  return minutes === 0 ? `${hours}h` : `${hours}h ${minutes}m`
}
