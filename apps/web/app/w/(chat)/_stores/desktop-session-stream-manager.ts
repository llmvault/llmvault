"use client"

import type { QueryClient } from "@tanstack/react-query"
import type { GoSessionStreamFrame } from "@/app/w/(chat)/_lib/go-session-stream"
import {
  applySessionStreamFrame,
  refreshSessionQueries,
} from "@/app/w/(chat)/_stores/session-stream-manager"
import { streamDesktopSession } from "@/lib/desktop/bridge"

interface DesktopStreamRecord {
  turnId: string
}

const desktopStreams = new Map<string, DesktopStreamRecord>()

export function ensureDesktopSessionStream(
  sessionId: string,
  turnId: string,
  queryClient: QueryClient
) {
  const existing = desktopStreams.get(sessionId)
  if (existing?.turnId === turnId) return

  const record = { turnId }
  desktopStreams.set(sessionId, record)
  void streamDesktopSession(sessionId, turnId, (frame) => {
    if (desktopStreams.get(sessionId) !== record) return
    applySessionStreamFrame(
      sessionId,
      queryClient,
      frame as GoSessionStreamFrame
    )
  })
    .catch(() => refreshSessionQueries(queryClient, sessionId))
    .finally(() => {
      if (desktopStreams.get(sessionId) === record) {
        desktopStreams.delete(sessionId)
      }
      void refreshSessionQueries(queryClient, sessionId)
    })
}
