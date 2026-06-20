"use client"

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react"
import { usePathname, useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import {
  Group,
  Panel,
  Separator,
  usePanelRef,
  type PanelImperativeHandle,
} from "react-resizable-panels"
import { animate, type AnimationPlaybackControls } from "motion/react"
import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"
import { useAuth } from "@/lib/auth/auth-context"
import { ChatHeader } from "@/app/w/(chat)/_components/chat-header"
import type { ChatHeaderAgent } from "@/app/w/(chat)/_components/chat-header-types"
import {
  RightPanel,
  type PanelViewID,
} from "@/app/w/(chat)/_components/right-panel"
import { Sidebar } from "@/app/w/(chat)/_components/sidebar"
import { LineCommentsProvider } from "@/app/w/(chat)/_components/line-comments"
import {
  agentById,
  DEFAULT_AGENT_ID,
  type Agent,
} from "@/app/w/(chat)/_lib/agents"
import {
  agentAvatarURL,
  agentDisplayName,
  agentIcon,
  agentModel,
  sessionDisplayName,
  sessionRouteFromPathname,
} from "@/app/w/(chat)/_lib/sidebar-data"
import { CHAT_QUERY_STALE_TIME_MS } from "@/app/w/(chat)/_lib/chat-cache"
import { hydrateSessionRuntimeFromResponse } from "@/app/w/(chat)/_stores/session-stream-manager"
import {
  getCachedSessionSandboxAccess,
  getSessionSandboxAccess,
  type SessionSandboxAccess,
} from "@/app/w/(chat)/_lib/session-sandbox-access"
import {
  selectSessionWorkspace,
  useSessionWorkspaceHydration,
  useSessionWorkspaceStore,
  type WorkspacePanelViewID,
} from "@/app/w/(chat)/_stores/session-workspace-store"

const SIDEBAR_WIDTH = 300
const RIGHT_SIZE = 42 // percent
const RIGHT_MAX_SIZE = 70 // percent
const PANEL_EASE = [0.32, 0.72, 0, 1] as const

// A chat session is pinned to one agent for its whole lifetime; only the
// model can change after creation, and only within the agent's model list.
export interface ChatSession {
  title: string
  agentId: string
  agentName?: string
  agentIcon?: string
  agentAvatarURL?: string
  modelId: string
  initialMessage?: string
  agentTurnStatus?: string
  agentTurnID?: string
  agentTurnStartedAt?: string
  lastTurnOutcome?: string
}

interface WorkspaceContextValue {
  session: ChatSession | null
  startNewChat: () => void
  openChannel: (channelSlug: string) => void
  openChat: (
    channelSlug: string,
    sessionId: string,
    session?: ChatSession,
    options?: { replace?: boolean }
  ) => void
  startSession: (
    agentId: string,
    firstMessage: string,
    modelId?: string
  ) => void
  setModel: (modelId: string) => void
  openView: (id: PanelViewID) => void
  sandboxAccess?: SessionSandboxAccess
  sandboxAccessPending: boolean
  sandboxAccessError: unknown
  refreshSandboxAccess: () => void
}

const WorkspaceContext = createContext<WorkspaceContextValue | null>(null)
type SessionResponse = components["schemas"]["sessionResponse"]
type AgentResponse = components["schemas"]["agentListItem"]
type SandboxRuntimeState = {
  sessionId: string
  access?: SessionSandboxAccess
  pending: boolean
  error: unknown
}

export function useWorkspace() {
  const context = useContext(WorkspaceContext)
  if (!context) {
    throw new Error("useWorkspace must be used inside WorkspaceShell")
  }
  return context
}

export function WorkspaceShell({ children }: { children: React.ReactNode }) {
  const router = useRouter()
  const pathname = usePathname()
  const queryClient = useQueryClient()
  const { user, activeOrg } = useAuth()
  const [sidebarOpen, setSidebarOpenState] = useState(true)
  const [rightOpen, setRightOpenState] = useState(false)
  const sidebarOpenRef = useRef(true)
  const rightOpenRef = useRef(false)
  const [draftSession, setDraftSession] = useState<ChatSession | null>(null)
  const [routePreviewSession, setRoutePreviewSession] = useState<{
    sessionId: string
    session: ChatSession
  } | null>(null)
  const [sandboxRuntimeState, setSandboxRuntimeState] =
    useState<SandboxRuntimeState | null>(null)
  const sandboxRuntimeRequestRef = useRef(0)

  const routeParams = useMemo(
    () => sessionRouteFromPathname(pathname),
    [pathname]
  )
  const routeSessionID = routeParams?.sessionId
  const routeIsOptimistic = routeSessionID?.startsWith("tmp_") ?? false
  const workspaceSessionKey = routeSessionID ?? "new-chat"
  useSessionWorkspaceHydration({
    orgId: activeOrg?.id,
    userId: user?.id,
  })
  const openViews = useSessionWorkspaceStore(
    (state) =>
      selectSessionWorkspace(state, workspaceSessionKey).rightPanel
        .openViews as PanelViewID[]
  )
  const activeView = useSessionWorkspaceStore(
    (state) =>
      selectSessionWorkspace(state, workspaceSessionKey).rightPanel
        .activeView as PanelViewID | null
  )
  const rightPanelOpen = useSessionWorkspaceStore(
    (state) =>
      selectSessionWorkspace(state, workspaceSessionKey).rightPanel.open
  )
  const rightMaximized = useSessionWorkspaceStore(
    (state) =>
      selectSessionWorkspace(state, workspaceSessionKey).rightPanel.maximized
  )
  const rightPanelSizePercent = useSessionWorkspaceStore(
    (state) =>
      selectSessionWorkspace(state, workspaceSessionKey).rightPanel
        .sizePercent || RIGHT_SIZE
  )
  const openPanelView = useSessionWorkspaceStore((state) => state.openPanelView)
  const closePanelView = useSessionWorkspaceStore(
    (state) => state.closePanelView
  )
  const setActivePanelView = useSessionWorkspaceStore(
    (state) => state.setActivePanelView
  )
  const setRightPanelOpen = useSessionWorkspaceStore(
    (state) => state.setRightPanelOpen
  )
  const setRightPanelMaximized = useSessionWorkspaceStore(
    (state) => state.setRightPanelMaximized
  )
  const setRightPanelSize = useSessionWorkspaceStore(
    (state) => state.setRightPanelSize
  )
  const routeSessionQuery = $api.useQuery(
    "get",
    "/v1/sessions/{id}",
    {
      params: {
        path: {
          id: routeSessionID ?? "",
        },
      },
    },
    {
      enabled: Boolean(routeSessionID) && !routeIsOptimistic,
      retry: false,
      staleTime: CHAT_QUERY_STALE_TIME_MS,
    }
  )
  const agentsQuery = $api.useQuery(
    "get",
    "/v1/agents",
    {
      params: {
        query: {
          status: "active",
          limit: 100,
        },
      },
    },
    {
      enabled: Boolean(routeSessionID),
      retry: false,
      staleTime: CHAT_QUERY_STALE_TIME_MS,
    }
  )
  const agentsByID = useMemo(
    () =>
      new Map(
        (agentsQuery.data?.data ?? []).flatMap((agent) =>
          agent.id ? ([[agent.id, agent]] as const) : []
        )
      ),
    [agentsQuery.data?.data]
  )
  const routeSession = useMemo(() => {
    if (!routeSessionID) return null
    const fetched = chatSessionFromResponse(
      routeSessionQuery.data?.session,
      agentsByID
    )
    const preview =
      routePreviewSession?.sessionId === routeSessionID
        ? routePreviewSession.session
        : null
    return (
      fetched ??
      preview ?? {
        title: routeSessionQuery.isError ? "Chat unavailable" : "Loading chat",
        agentId: DEFAULT_AGENT_ID,
        modelId: agentById(DEFAULT_AGENT_ID).defaultModelId,
      }
    )
  }, [
    agentsByID,
    routeSessionID,
    routePreviewSession,
    routeSessionQuery.data?.session,
    routeSessionQuery.isError,
  ])
  const session = routeSession ?? draftSession
  const routeSandboxID = routeSessionQuery.data?.session?.sandbox_id ?? null

  const sidebarPanelRef = usePanelRef()
  const rightPanelRef = usePanelRef()
  const sidebarAnim = useRef<AnimationPlaybackControls | null>(null)
  const rightAnim = useRef<AnimationPlaybackControls | null>(null)
  const rightProgrammaticResizeRef = useRef(false)
  const rightPanelOpenRef = useRef(rightPanelOpen)
  const rightPanelSizePercentRef = useRef(RIGHT_SIZE)
  const workspaceSessionKeyRef = useRef(workspaceSessionKey)
  const rightPanelSizePersistTimer = useRef<ReturnType<
    typeof setTimeout
  > | null>(null)
  const sidebarWasOpenRef = useRef(true)

  useEffect(() => {
    workspaceSessionKeyRef.current = workspaceSessionKey
  }, [workspaceSessionKey])

  useEffect(() => {
    rightPanelOpenRef.current = rightPanelOpen
  }, [rightPanelOpen])

  useEffect(() => {
    rightPanelSizePercentRef.current = rightPanelSizePercent
  }, [rightPanelSizePercent])

  const updateSidebarOpen = useCallback((open: boolean) => {
    if (sidebarOpenRef.current === open) return
    sidebarOpenRef.current = open
    setSidebarOpenState(open)
  }, [])

  const updateRightOpen = useCallback((open: boolean) => {
    if (rightOpenRef.current === open) return
    rightOpenRef.current = open
    setRightOpenState(open)
  }, [])

  const animatePanel = useCallback(
    (
      handle: PanelImperativeHandle | null,
      anim: React.RefObject<AnimationPlaybackControls | null>,
      to: number,
      unit: "px" | "%",
      onComplete?: () => void
    ) => {
      if (!handle) {
        onComplete?.()
        return
      }
      anim.current?.stop()
      const size = handle.getSize()
      const from = unit === "px" ? size.inPixels : size.asPercentage
      anim.current = animate(from, to, {
        duration: 0.3,
        ease: PANEL_EASE,
        onUpdate: (value) => handle.resize(unit === "px" ? value : `${value}%`),
        onComplete,
      })
    },
    []
  )

  const toggleSidebar = useCallback(() => {
    const isOpen = sidebarOpenRef.current
    animatePanel(
      sidebarPanelRef.current,
      sidebarAnim,
      isOpen ? 0 : SIDEBAR_WIDTH,
      "px"
    )
  }, [animatePanel, sidebarPanelRef])

  const setRightSize = useCallback(
    (percent: number) => {
      rightProgrammaticResizeRef.current = true
      animatePanel(rightPanelRef.current, rightAnim, percent, "%", () => {
        rightProgrammaticResizeRef.current = false
      })
    },
    [animatePanel, rightPanelRef]
  )

  const scheduleRightPanelSizePersist = useCallback(
    (percent: number) => {
      rightPanelSizePercentRef.current = percent
      if (rightPanelSizePersistTimer.current) {
        clearTimeout(rightPanelSizePersistTimer.current)
      }
      const sessionKey = workspaceSessionKeyRef.current
      rightPanelSizePersistTimer.current = setTimeout(() => {
        rightPanelSizePersistTimer.current = null
        setRightPanelSize(sessionKey, percent)
      }, 200)
    },
    [setRightPanelSize]
  )

  useEffect(
    () => () => {
      if (rightPanelSizePersistTimer.current) {
        clearTimeout(rightPanelSizePersistTimer.current)
      }
    },
    []
  )

  const toggleRight = useCallback(() => {
    const nextOpen = !rightPanelOpen
    setRightPanelMaximized(workspaceSessionKey, false)
    setRightPanelOpen(workspaceSessionKey, nextOpen)
  }, [
    rightPanelOpen,
    setRightPanelOpen,
    setRightPanelMaximized,
    workspaceSessionKey,
  ])

  const toggleMaximize = useCallback(() => {
    const next = !rightMaximized
    setRightPanelOpen(workspaceSessionKey, true)
    setRightPanelMaximized(workspaceSessionKey, next)
    if (next) {
      // Maximizing the right panel takes the sidebar's room; remember
      // whether it was open so restoring brings it back.
      const sidebarOpenNow = sidebarOpenRef.current
      sidebarWasOpenRef.current = sidebarOpenNow
      if (sidebarOpenNow) {
        animatePanel(sidebarPanelRef.current, sidebarAnim, 0, "px")
      }
    } else if (sidebarWasOpenRef.current && !sidebarOpenRef.current) {
      animatePanel(sidebarPanelRef.current, sidebarAnim, SIDEBAR_WIDTH, "px")
    }
  }, [
    animatePanel,
    rightMaximized,
    setRightPanelOpen,
    setRightPanelMaximized,
    sidebarPanelRef,
    workspaceSessionKey,
  ])

  const openView = useCallback(
    (id: PanelViewID) => {
      openPanelView(workspaceSessionKey, id as WorkspacePanelViewID)
    },
    [openPanelView, workspaceSessionKey]
  )

  const closeView = useCallback(
    (id: PanelViewID) => {
      closePanelView(workspaceSessionKey, id as WorkspacePanelViewID)
    },
    [closePanelView, workspaceSessionKey]
  )

  const selectView = useCallback(
    (id: PanelViewID) =>
      setActivePanelView(workspaceSessionKey, id as WorkspacePanelViewID),
    [setActivePanelView, workspaceSessionKey]
  )

  const startNewChat = useCallback(() => {
    setDraftSession(null)
    setRoutePreviewSession(null)
    router.push("/w")
  }, [router])

  const openChannel = useCallback(
    (channelSlug: string) => {
      setDraftSession(null)
      setRoutePreviewSession(null)
      router.push(`/w/channels/${channelSlug}`)
    },
    [router]
  )

  const openChat = useCallback(
    (
      channelSlug: string,
      sessionId: string,
      session?: ChatSession,
      options: { replace?: boolean } = {}
    ) => {
      setDraftSession(null)
      setRoutePreviewSession(session ? { sessionId, session } : null)
      const href = `/w/channels/${channelSlug}/${sessionId}`
      if (options.replace) {
        router.replace(href)
      } else {
        router.push(href)
      }
    },
    [router]
  )

  const startSession = useCallback(
    (agentId: string, firstMessage: string, modelId?: string) => {
      const agent = agentById(agentId)
      const title =
        firstMessage.length > 44
          ? `${firstMessage.slice(0, 44).trimEnd()}…`
          : firstMessage
      setDraftSession({
        title,
        agentId,
        modelId:
          modelId && agent.modelIds.includes(modelId)
            ? modelId
            : agent.defaultModelId,
        initialMessage: firstMessage,
      })
    },
    []
  )

  const setModel = useCallback((modelId: string) => {
    setDraftSession((current) => {
      if (!current) return current
      const agent = safeStaticAgentById(current.agentId)
      if (agent && !agent.modelIds.includes(modelId)) return current
      return { ...current, modelId }
    })
  }, [])

  const runtimeAccessNeeded =
    Boolean(routeSessionID) &&
    !routeIsOptimistic &&
    rightPanelOpen &&
    panelViewNeedsRuntimeAccess(activeView)
  const sandboxRuntimeStateMatches =
    sandboxRuntimeState?.sessionId === routeSessionID
  const sandboxAccess = sandboxRuntimeStateMatches
    ? sandboxRuntimeState?.access
    : undefined
  const sandboxRuntimeReady =
    runtimeAccessNeeded &&
    sandboxAccess?.session_id === routeSessionID &&
    Boolean(sandboxAccess?.sandbox_base_url) &&
    Boolean(sandboxAccess?.token)
  const sandboxRuntimeError = sandboxRuntimeStateMatches
    ? sandboxRuntimeState?.error
    : null
  const sandboxRuntimePending =
    runtimeAccessNeeded &&
    !sandboxRuntimeReady &&
    !sandboxRuntimeError
  const sandboxAccessPendingForSession = sandboxRuntimePending

  const loadSandboxAccess = useCallback((options: { force?: boolean } = {}) => {
    if (!routeSessionID || routeIsOptimistic) return
    const requestID = sandboxRuntimeRequestRef.current + 1
    sandboxRuntimeRequestRef.current = requestID
    const cached = options.force
      ? null
      : getCachedSessionSandboxAccess(routeSessionID, {
          expectedSandboxId: routeSandboxID,
        })
    if (cached) {
      setSandboxRuntimeState({
        sessionId: routeSessionID,
        access: cached,
        pending: false,
        error: null,
      })
      return
    }
    setSandboxRuntimeState({
      sessionId: routeSessionID,
      pending: true,
      error: null,
    })
    void (async () => {
      try {
        const access = await getSessionSandboxAccess(routeSessionID, {
          expectedSandboxId: routeSandboxID,
          force: options.force,
        })
        if (sandboxRuntimeRequestRef.current !== requestID) return
        setSandboxRuntimeState({
          sessionId: routeSessionID,
          access,
          pending: false,
          error: null,
        })
      } catch (error) {
        if (sandboxRuntimeRequestRef.current !== requestID) return
        setSandboxRuntimeState({
          sessionId: routeSessionID,
          pending: false,
          error,
        })
      }
    })()
  }, [routeIsOptimistic, routeSandboxID, routeSessionID])

  const refreshSandboxAccess = useCallback(() => {
    loadSandboxAccess({ force: true })
  }, [loadSandboxAccess])

  useEffect(() => {
    if (!routeSessionID || routeIsOptimistic || !runtimeAccessNeeded) {
      sandboxRuntimeRequestRef.current += 1
      if (!routeSessionID || routeIsOptimistic) {
        window.queueMicrotask(() => setSandboxRuntimeState(null))
      } else {
        window.queueMicrotask(() =>
          setSandboxRuntimeState((current) =>
            current?.sessionId === routeSessionID && current.pending
              ? { ...current, pending: false }
              : current
          )
        )
      }
      return
    }
    window.queueMicrotask(() => loadSandboxAccess())
  }, [
    loadSandboxAccess,
    routeIsOptimistic,
    routeSessionID,
    runtimeAccessNeeded,
  ])

  useEffect(() => {
    hydrateSessionRuntimeFromResponse(routeSessionQuery.data?.session, queryClient)
  }, [queryClient, routeSessionQuery.data?.session])

  useEffect(() => {
    const nextSize = rightPanelOpen
      ? rightMaximized
        ? RIGHT_MAX_SIZE
        : rightPanelSizePercentRef.current
      : 0
    setRightSize(nextSize)
  }, [
    rightMaximized,
    rightPanelOpen,
    setRightSize,
    workspaceSessionKey,
  ])

  const workspace = useMemo(
    () => ({
      session,
      startNewChat,
      openChannel,
      openChat,
      startSession,
      setModel,
      openView,
      sandboxAccess,
      sandboxAccessPending: sandboxAccessPendingForSession,
      sandboxAccessError: sandboxRuntimeError,
      refreshSandboxAccess,
    }),
    [
      session,
      startNewChat,
      openChannel,
      openChat,
      startSession,
      setModel,
      openView,
      sandboxAccess,
      sandboxAccessPendingForSession,
      sandboxRuntimeError,
      refreshSandboxAccess,
    ]
  )
  const headerAgent = useMemo(
    () => (session ? chatHeaderAgent(session) : null),
    [session]
  )

  return (
    <WorkspaceContext.Provider value={workspace}>
      <LineCommentsProvider scopeKey={routeSessionID ?? "new-chat"}>
        <div className="bg-surface h-screen w-screen overflow-hidden text-foreground">
          <Group orientation="horizontal" className="h-full w-full">
            <Panel
              id="sidebar"
              panelRef={sidebarPanelRef}
              defaultSize={SIDEBAR_WIDTH}
              minSize={0}
              maxSize={420}
              className="min-w-0 overflow-hidden"
              onResize={(size) => updateSidebarOpen(size.inPixels > 8)}
            >
              <div className="h-full min-w-[230px]">
                <Sidebar onCollapse={toggleSidebar} />
              </div>
            </Panel>
            <Separator
              className={`shrink-0 bg-border transition-colors hover:bg-accent data-[resizing]:bg-accent ${
                sidebarOpen ? "w-px" : "w-0"
              }`}
            />

            <Panel id="chat" minSize={480} className="min-w-0">
              <div className="flex h-full min-w-0 flex-col">
                <ChatHeader
                  title={session?.title ?? "New chat"}
                  agent={headerAgent}
                  sidebarOpen={sidebarOpen}
                  onExpandSidebar={toggleSidebar}
                  rightOpen={rightPanelOpen || rightOpen}
                  onToggleRight={toggleRight}
                />
                <div className="relative min-h-0 flex-1">
                  {children}
                </div>
              </div>
            </Panel>

            <Separator
              className={`shrink-0 bg-border transition-colors hover:bg-accent data-[resizing]:bg-accent ${
                rightPanelOpen || rightOpen ? "w-px" : "w-0"
              }`}
            />
            <Panel
              id="side"
              panelRef={rightPanelRef}
              defaultSize={0}
              minSize={0}
              className="min-w-0 overflow-hidden"
              onResize={(size) => {
                const open = size.inPixels > 8
                const programmatic = rightProgrammaticResizeRef.current
                updateRightOpen(open)
                if (!programmatic && open !== rightPanelOpenRef.current) {
                  rightPanelOpenRef.current = open
                  setRightPanelOpen(workspaceSessionKey, open)
                }
                if (open && !rightMaximized && !programmatic) {
                  scheduleRightPanelSizePersist(size.asPercentage)
                }
              }}
            >
              <div className="h-full min-w-90">
                <RightPanel
                  sessionId={routeSessionID}
                  sandboxAccess={sandboxAccess}
                  sandboxAccessPending={sandboxAccessPendingForSession}
                  sandboxAccessError={sandboxRuntimeError}
                  onRefreshSandboxAccess={refreshSandboxAccess}
                  openViews={openViews}
                  activeView={activeView}
                  maximized={rightMaximized}
                  onSelectView={selectView}
                  onOpenView={openView}
                  onCloseView={closeView}
                  onToggleMaximize={toggleMaximize}
                  onClosePanel={toggleRight}
                />
              </div>
            </Panel>
          </Group>
        </div>
      </LineCommentsProvider>
    </WorkspaceContext.Provider>
  )
}

function chatSessionFromResponse(
  session?: SessionResponse,
  agentsByID?: Map<string, AgentResponse>
): ChatSession | null {
  if (!session) return null
  const agentID = session.agent_id?.trim() || DEFAULT_AGENT_ID
  const apiAgent = agentsByID?.get(agentID)
  const staticAgent = safeStaticAgentById(agentID)
  const fallbackAgent = staticAgent ?? agentById(DEFAULT_AGENT_ID)
  return {
    title: sessionDisplayName(session),
    agentId: agentID,
    agentName: apiAgent ? agentDisplayName(apiAgent) : staticAgent?.name,
    agentIcon: apiAgent ? agentIcon(apiAgent) : staticAgent?.icon,
    agentAvatarURL: agentAvatarURL(apiAgent),
    modelId:
      session.model?.trim() ||
      agentModel(apiAgent) ||
      fallbackAgent.defaultModelId,
    agentTurnStatus: session.agent_turn_status,
    agentTurnID: session.agent_turn_id,
    agentTurnStartedAt: session.agent_turn_started_at,
    lastTurnOutcome: session.last_turn_outcome,
  }
}

function safeStaticAgentById(id: string): Agent | null {
  try {
    return agentById(id)
  } catch {
    return null
  }
}

function safeAgentById(id: string): Agent {
  return safeStaticAgentById(id) ?? agentById(DEFAULT_AGENT_ID)
}

function chatHeaderAgent(session: ChatSession): ChatHeaderAgent {
  const fallback = safeAgentById(session.agentId)
  return {
    name: session.agentName ?? fallback.name,
    icon: session.agentIcon ?? fallback.icon,
    avatarURL: session.agentAvatarURL,
  }
}

function panelViewNeedsRuntimeAccess(view: PanelViewID | null) {
  return view === "review" || view === "files"
}
