"use client"

import { fetchEventSource } from "@microsoft/fetch-event-source"
import { api } from "@/lib/api/client"
import { extractErrorMessage } from "@/lib/api/error"
import type { components } from "@/lib/api/schema"

export type WebSession = components["schemas"]["employeeSessionResponse"]
export type WebSessionEvent =
  components["schemas"]["employeeSessionEventResponse"] & { payload?: unknown }

export type Paginated<T> = {
  data?: T[]
  next_cursor?: string | null
  has_more?: boolean
}

export type SendSessionMessageResult =
  components["schemas"]["sendEmployeeSessionMessageResponse"]

export type PendingSessionStream = {
  sessionID: string
  text: string
  streamURL: string
  createdAt: number
}

const pendingStreamPrefix = "hivy.web.pending-stream:"

export async function fetchEmployeeID() {
  const { data, error } = await api.GET("/v1/employees", {
    params: { query: { limit: 1 } },
  })
  if (error) {
    throw new Error(extractErrorMessage(error, "Failed to load Hivy"))
  }
  const employeeID = data?.data?.[0]?.id
  if (!employeeID) {
    throw new Error("No Hivy employee is available")
  }
  return employeeID
}

export async function fetchWebSessions(
  employeeID: string,
  cursor?: string
): Promise<Paginated<WebSession>> {
  const { data, error } = await api.GET("/v1/employees/{id}/sessions", {
    params: {
      path: { id: employeeID },
      query: { limit: 100, cursor },
    },
  })
  if (error) {
    throw new Error(extractErrorMessage(error, "Failed to load sessions"))
  }
  const page = data as Paginated<WebSession>
  return {
    ...page,
    data: (page.data ?? []).filter((session) => session.source === "web"),
  }
}

export async function fetchSessionEvents(
  employeeID: string,
  sessionID: string,
  cursor?: string
): Promise<Paginated<WebSessionEvent>> {
  const { data, error } = await api.GET(
    "/v1/employees/{id}/sessions/{sessionID}/events",
    {
      params: {
        path: { id: employeeID, sessionID },
        query: { limit: 80, cursor },
      },
    }
  )
  if (error) {
    throw new Error(extractErrorMessage(error, "Failed to load messages"))
  }
  return data as Paginated<WebSessionEvent>
}

export async function sendSessionMessage({
  employeeID,
  sessionID,
  text,
}: {
  employeeID: string
  sessionID?: string
  text: string
}) {
  const { data, error } = await api.POST(
    "/v1/employees/{id}/sessions/messages",
    {
      params: { path: { id: employeeID } },
      body: {
        text,
        session_id: sessionID,
      },
    }
  )
  if (error) {
    throw new Error(extractErrorMessage(error, "Failed to send message"))
  }
  if (!data?.employee_session_id || !data.response_stream_url) {
    throw new Error("Hivy did not return a response stream")
  }
  return data
}

export function savePendingStream(pending: PendingSessionStream) {
  if (typeof window === "undefined") return
  window.sessionStorage.setItem(
    pendingStreamPrefix + pending.sessionID,
    JSON.stringify(pending)
  )
}

export function takePendingStream(sessionID: string) {
  if (typeof window === "undefined") return null
  const key = pendingStreamPrefix + sessionID
  const raw = window.sessionStorage.getItem(key)
  if (!raw) return null
  window.sessionStorage.removeItem(key)
  try {
    const pending = JSON.parse(raw) as PendingSessionStream
    return pending.sessionID === sessionID ? pending : null
  } catch {
    return null
  }
}

export async function streamSessionResponse({
  streamURL,
  signal,
  onText,
}: {
  streamURL: string
  signal: AbortSignal
  onText: (text: string, mode: "append" | "replace") => void
}) {
  let hasTokenText = false
  await fetchEventSource(streamURL, {
    method: "GET",
    headers: { Accept: "text/event-stream" },
    credentials: "include",
    signal,
    openWhenHidden: true,
    async onopen(response) {
      const contentType = response.headers.get("content-type") ?? ""
      if (response.ok && contentType.includes("text/event-stream")) return
      throw new Error(`Stream failed with HTTP ${response.status}`)
    },
    onmessage(event) {
      if (!event.data || event.data === "keep-alive") return
      const text = textFromStreamPayload(event.data)
      if (!text) return
      if (event.event === "final" && hasTokenText) return
      if (event.event === "final") {
        onText(text, "replace")
        hasTokenText = true
        return
      }
      if (event.event === "token" || event.event === "thinking") {
        hasTokenText = true
        onText(text, "append")
        return
      }
      onText(text, "append")
    },
    onerror(error) {
      throw error
    },
  })
}

export function sessionTitle(session: WebSession) {
  return (
    session.name ||
    session.source_resource_key ||
    session.runtime_conversation_id ||
    "Untitled session"
  )
}

export function eventText(event: WebSessionEvent) {
  const payload = decodePayload(event.payload)
  return (
    payloadString(payload, "text") ||
    payloadString(payload, "message") ||
    payloadString(payload, "content") ||
    payloadString(payload, "markdown") ||
    nestedPayloadString(payload, ["agent_event", "text"]) ||
    nestedPayloadString(payload, ["agent_event", "content"]) ||
    ""
  )
}

export function eventRole(event: WebSessionEvent): "user" | "assistant" | null {
  const type = event.event_type ?? ""
  if (type === "user.message.received" || type === "message_received")
    return "user"
  if (type === "agent.message.sent" || type === "response_completed") {
    return "assistant"
  }
  return null
}

function textFromStreamPayload(data: string) {
  try {
    const payload = JSON.parse(data)
    return (
      payloadString(payload, "text") ||
      payloadString(payload, "delta") ||
      payloadString(payload, "content") ||
      payloadString(payload, "markdown")
    )
  } catch {
    return data
  }
}

function decodePayload(payload: unknown) {
  if (!Array.isArray(payload)) return payload
  if (!payload.every((item) => typeof item === "number")) return payload
  try {
    return JSON.parse(new TextDecoder().decode(new Uint8Array(payload)))
  } catch {
    return payload
  }
}

function payloadString(payload: unknown, key: string) {
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    return ""
  }
  const value = (payload as Record<string, unknown>)[key]
  return typeof value === "string" ? value : ""
}

function nestedPayloadString(payload: unknown, path: string[]) {
  let value = payload
  for (const key of path) {
    if (!value || typeof value !== "object" || Array.isArray(value)) {
      return ""
    }
    value = (value as Record<string, unknown>)[key]
  }
  return typeof value === "string" ? value : ""
}
