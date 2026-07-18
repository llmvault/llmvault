"use client"

import { useEffect, useState } from "react"
import { usePathname } from "next/navigation"
import { AnimatePresence, motion } from "motion/react"
import { useWorkspace, type ChatSession } from "./shell"
import {
  agentAvatarURL,
  agentDisplayName,
  agentIcon,
  sessionActivityLabel,
  sessionDisplayName,
  type SidebarAgentResponse,
  type SidebarSessionResponse,
} from "@/app/w/(chat)/_lib/sidebar-data"
import { ChatRow, type SidebarSessionAgent } from "./sidebar-chat-row"
import { useQueryClient } from "@tanstack/react-query"
import { hydrateSessionListRuntime } from "@/app/w/(chat)/_stores/session-stream-manager"

const COLLAPSE_TRANSITION = {
  duration: 0.25,
  ease: [0.32, 0.72, 0, 1] as const,
}

export function SidebarTeamSessionGroup({
  name,
  sessions,
  agentsByID,
  onRenameSession,
  onShareSession,
  onArchiveSession,
}: {
  name: string
  sessions: SidebarSessionResponse[]
  agentsByID: Map<string, SidebarAgentResponse>
  onRenameSession: (sessionId: string, name: string) => void
  onShareSession: (sessionId: string) => void
  onArchiveSession: (sessionId: string) => void
}) {
  const { openChat } = useWorkspace()
  const pathname = usePathname()
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(true)

  useEffect(() => {
    hydrateSessionListRuntime(sessions, queryClient)
  }, [queryClient, sessions])

  return (
    <div className="flex flex-col">
      <button
        type="button"
        aria-expanded={open}
        aria-label={`${open ? "Collapse" : "Expand"} ${name}`}
        onClick={() => setOpen((value) => !value)}
        className="min-w-0 truncate px-3 pt-2 pb-1 text-left text-xs text-muted uppercase select-none"
      >
        {name}
      </button>
      <AnimatePresence initial={false}>
        {open ? (
          <motion.div
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: "auto", opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={COLLAPSE_TRANSITION}
            className="overflow-hidden"
          >
            <div className="flex flex-col gap-0.5">
              {sessions.length ? (
                sessions.map((session) => {
                  const id = session.id ?? ""
                  const agent = agentForSession(session, agentsByID)
                  return (
                    <ChatRow
                      key={id}
                      sessionId={id}
                      title={sessionDisplayName(session)}
                      agent={sidebarSessionAgent(agent)}
                      meta={sessionActivityLabel(session)}
                      active={pathname === `/w/sessions/${id}`}
                      onRename={
                        id
                          ? () =>
                              onRenameSession(id, sessionDisplayName(session))
                          : undefined
                      }
                      onShare={id ? () => onShareSession(id) : undefined}
                      onArchive={id ? () => onArchiveSession(id) : undefined}
                      onSelect={() =>
                        openChat(id, chatSessionFromResponse(session, agent))
                      }
                    />
                  )
                })
              ) : (
                <IndentedStatusRow label="No chats" muted />
              )}
            </div>
          </motion.div>
        ) : null}
      </AnimatePresence>
    </div>
  )
}

function agentForSession(
  session: SidebarSessionResponse,
  agentsByID: Map<string, SidebarAgentResponse>
) {
  const agentID = session.agent_id?.trim()
  return agentID ? agentsByID.get(agentID) : undefined
}

function sidebarSessionAgent(
  agent?: SidebarAgentResponse
): SidebarSessionAgent {
  return {
    name: agent ? agentDisplayName(agent) : "Agent",
    icon: agentIcon(agent),
    avatarURL: agentAvatarURL(agent),
  }
}

function chatSessionFromResponse(
  session: SidebarSessionResponse,
  agent?: SidebarAgentResponse
): ChatSession {
  return {
    title: sessionDisplayName(session),
    agentId: session.agent_id ?? "",
    agentName: agent ? agentDisplayName(agent) : undefined,
    agentIcon: agentIcon(agent),
    agentAvatarURL: agentAvatarURL(agent),
    modelId: session.model ?? agent?.model ?? "",
    agentTurnStatus: session.agent_turn_status,
    agentTurnID: session.agent_turn_id,
    agentTurnStartedAt: session.agent_turn_started_at,
    lastTurnOutcome: session.last_turn_outcome,
    source: session.source,
    sourceResourceKey: session.source_resource_key,
    loaded: true,
  }
}

function IndentedStatusRow({
  label,
  muted = false,
}: {
  label: string
  muted?: boolean
}) {
  return (
    <div
      className={`flex items-center gap-2 rounded-lg py-1.5 pr-3 pl-9 text-sm ${
        muted ? "text-muted/60" : "text-muted"
      }`}
    >
      <span className="min-w-0 flex-1 truncate">{label}</span>
    </div>
  )
}
