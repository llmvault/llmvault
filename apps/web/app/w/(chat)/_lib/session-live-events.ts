import type { CodeLineCommentPayload } from "@/app/w/(chat)/_lib/code-line-comments"
import type { ImageAttachmentMetadata } from "@/app/w/(chat)/_lib/image-attachments"
import type { SessionEventResponse } from "@/app/w/(chat)/_lib/session-history"
import { useSessionRuntimeStore } from "@/app/w/(chat)/_stores/session-runtime-store"

export function appendLiveSessionEvent(
  sessionId: string,
  event: SessionEventResponse
) {
  const store = useSessionRuntimeStore.getState()
  store.setLiveEvents(sessionId, [
    ...(store.liveEventsBySessionId[sessionId] ?? []),
    event,
  ])
}

export function removeLiveSessionEvent(sessionId: string, eventID: string) {
  const store = useSessionRuntimeStore.getState()
  store.setLiveEvents(
    sessionId,
    (store.liveEventsBySessionId[sessionId] ?? []).filter(
      (event) => event.id !== eventID && event.event_id !== eventID
    )
  )
}

export function optimisticUserMessageEvent(
  sessionId: string,
  text: string,
  attachments: ImageAttachmentMetadata[],
  codeLineComments: CodeLineCommentPayload[]
): SessionEventResponse {
  const eventID = `client:${clientID()}`
  return {
    id: eventID,
    event_id: eventID,
    event_type: "user.message.received",
    event_at: new Date().toISOString(),
    session_id: sessionId,
    source: "web",
    durability: "durable",
    sequence_number: 0,
    payload: {
      text,
      ...(attachments.length ? { attachments } : {}),
      ...(codeLineComments.length
        ? { code_line_comments: codeLineComments }
        : {}),
    },
  }
}

export function latestTurnID(events: SessionEventResponse[]) {
  for (let index = events.length - 1; index >= 0; index -= 1) {
    const payload = events[index].payload
    if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
      continue
    }
    const turnID = (payload as Record<string, unknown>).turn_id
    if (typeof turnID === "string" && turnID.trim()) {
      return turnID.trim()
    }
  }
  return undefined
}

function clientID() {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID()
  }
  return `${Date.now()}-${Math.random().toString(16).slice(2)}`
}
