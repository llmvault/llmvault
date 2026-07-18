"use client"

import { SessionView } from "@/app/w/(chat)/_components/new-session-view"
import { SessionThreadView } from "@/app/w/(chat)/_components/session-view"
import { useWorkspace } from "@/app/w/(chat)/_components/shell"

export function ChatCanvas({
  sessionId,
}: {
  sessionId?: string
}) {
  const { session, openChat } = useWorkspace()

  if (!session) {
    return <SessionView onSessionCreated={openChat} />
  }

  return (
    <SessionThreadView
      key={sessionId ?? session.title}
      session={session}
      sessionId={sessionId}
    />
  )
}
