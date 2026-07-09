"use client"

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
} from "react"
import { usePathname, useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import {
  Group,
  Panel,
  Separator,
  usePanelRef,
  type PanelImperativeHandle,
  type PanelSize,
} from "react-resizable-panels"
import { animate, type AnimationPlaybackControls } from "motion/react"
import { toast } from "@heroui/react"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"
import { useAuth } from "@/lib/auth/auth-context"
import { ChatHeader } from "@/app/w/(chat)/_components/chat-header"
import type { ChatHeaderAgent } from "@/app/w/(chat)/_components/chat-header-types"
import {
  RenameSessionModal,
  type RenameSessionTarget,
} from "@/app/w/(chat)/_components/rename-session-modal"
import { ShareSessionModal } from "@/app/w/(chat)/_components/share-session-modal"
import {
  RightPanel,
  type PanelViewID,
} from "@/app/w/(chat)/_components/right-panel"
import { Sidebar } from "@/app/w/(chat)/_components/sidebar"
import { LineCommentsProvider } from "@/app/w/(chat)/_components/line-comments"
import {
  agentAvatarURL,
  agentDisplayName,
  agentIcon,
  agentModel,
  sessionDisplayName,
  sessionRouteFromPathname,
} from "@/app/w/(chat)/_lib/sidebar-data"
import {
  CHAT_QUERY_STALE_TIME_MS,
  invalidateSessionListQueries,
  removeSessionFromChannelCache,
} from "@/app/w/(chat)/_lib/chat-cache"
import {
  DEFAULT_SIDEBAR_PREFERENCES,
  SIDEBAR_COLLAPSED_THRESHOLD,
  SIDEBAR_MAX_WIDTH,
  SIDEBAR_MIN_OPEN_WIDTH,
  SIDEBAR_PREFERENCES_DEBOUNCE_MS,
  clampSidebarWidth,
  loadSidebarPreferences,
  saveSidebarPreferences,
  type SidebarPreferences,
} from "@/app/w/(chat)/_lib/sidebar-preferences"
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

const RIGHT_SIZE = 42 // percent
const RIGHT_MAX_SIZE = 70 // percent
const RIGHT_PANEL_COLLAPSED_THRESHOLD_PX = 8
const RIGHT_PANEL_SIZE_EPSILON = 0.1
const PANEL_EASE = [0.32, 0.72, 0, 1] as const

// A chat session is pinned to one agent for its whole lifetime; only the
// model can change after creation.
export interface ChatSession {
  title: string
  agentId: string
  agentName?: string
  agentIcon?: string
  agentAvatarURL?: string
  modelId: string
  agentTurnStatus?: string
  agentTurnID?: string
  agentTurnStartedAt?: string
  lastTurnOutcome?: string
  source?: string
  sourceResourceKey?: string
  loaded?: boolean
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
  const { user, activeOrg, isLoading: authLoading } = useAuth()
  const [sidebarOpen, setSidebarOpenState] = useState(
    DEFAULT_SIDEBAR_PREFERENCES.open
  )
  const [rightOpen, setRightOpenState] = useState(false)
  const sidebarOpenRef = useRef(DEFAULT_SIDEBAR_PREFERENCES.open)
  const rightOpenRef = useRef(false)
  const [renameSession, setRenameSession] =
    useState<RenameSessionTarget | null>(null)
  const [shareSessionID, setShareSessionID] = useState<string | null>(null)
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
  const routeIsTemporarySession = routeSessionID?.startsWith("tmp_") ?? false
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
      enabled: Boolean(routeSessionID) && !routeIsTemporarySession,
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
        agentId: "",
        modelId: "",
        loaded: false,
      }
    )
  }, [
    agentsByID,
    routeSessionID,
    routePreviewSession,
    routeSessionQuery.data?.session,
    routeSessionQuery.isError,
  ])
  const session = routeSession
  const routeSandboxID = routeSessionQuery.data?.session?.sandbox_id ?? null

  const sidebarPanelRef = usePanelRef()
  const rightPanelRef = usePanelRef()
  const sidebarAnim = useRef<AnimationPlaybackControls | null>(null)
  const rightAnim = useRef<AnimationPlaybackControls | null>(null)
  const sidebarWidthRef = useRef(DEFAULT_SIDEBAR_PREFERENCES.width)
  const sidebarResizePersistDisabledRef = useRef(false)
  const sidebarResizeWidthCaptureDisabledRef = useRef(false)
  const sidebarPreferencesReadyRef = useRef(false)
  const sidebarPreferencesPersistTimer = useRef<ReturnType<
    typeof setTimeout
  > | null>(null)
  const rightManualResizeRef = useRef(false)
  const rightSkipNextSyncRef = useRef(false)
  const rightPanelOpenRef = useRef(rightPanelOpen)
  const rightPanelSizePercentRef = useRef(RIGHT_SIZE)
  const workspaceSessionKeyRef = useRef(workspaceSessionKey)
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

  const scheduleSidebarPreferencesPersist = useCallback(
    (preferences: SidebarPreferences) => {
      if (sidebarPreferencesPersistTimer.current) {
        clearTimeout(sidebarPreferencesPersistTimer.current)
      }
      const userId = user?.id
      const normalized = {
        open: preferences.open,
        width: clampSidebarWidth(preferences.width),
      }
      sidebarPreferencesPersistTimer.current = setTimeout(() => {
        sidebarPreferencesPersistTimer.current = null
        saveSidebarPreferences({ userId }, normalized)
      }, SIDEBAR_PREFERENCES_DEBOUNCE_MS)
    },
    [user?.id]
  )

  const updateSidebarWidthFromResize = useCallback((size: PanelSize) => {
    if (size.inPixels >= SIDEBAR_MIN_OPEN_WIDTH) {
      sidebarWidthRef.current = clampSidebarWidth(size.inPixels)
    }
  }, [])

  const handleSidebarResize = useCallback(
    (size: PanelSize) => {
      const open = size.inPixels > SIDEBAR_COLLAPSED_THRESHOLD
      updateSidebarOpen(open)
      if (!sidebarResizeWidthCaptureDisabledRef.current) {
        updateSidebarWidthFromResize(size)
      }
      if (
        !sidebarPreferencesReadyRef.current ||
        sidebarResizePersistDisabledRef.current
      ) {
        return
      }
      scheduleSidebarPreferencesPersist({
        open,
        width: sidebarWidthRef.current,
      })
    },
    [
      scheduleSidebarPreferencesPersist,
      updateSidebarOpen,
      updateSidebarWidthFromResize,
    ]
  )

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

  const resizeSidebar = useCallback(
    (width: number, options: { persist: boolean }) => {
      const applyResize = () =>
        animatePanel(sidebarPanelRef.current, sidebarAnim, width, "px", () => {
          sidebarResizeWidthCaptureDisabledRef.current = false
          if (!options.persist) {
            sidebarResizePersistDisabledRef.current = false
          }
        })
      if (options.persist) {
        sidebarResizePersistDisabledRef.current = false
        sidebarResizeWidthCaptureDisabledRef.current = true
        applyResize()
        return
      }
      sidebarResizePersistDisabledRef.current = true
      sidebarResizeWidthCaptureDisabledRef.current = true
      applyResize()
    },
    [animatePanel, sidebarPanelRef]
  )

  useEffect(() => {
    if (!user?.id && authLoading) return
    const preferences = loadSidebarPreferences({ userId: user?.id })
    sidebarPreferencesReadyRef.current = true
    sidebarWidthRef.current = preferences.width
    sidebarOpenRef.current = preferences.open
    window.queueMicrotask(() => setSidebarOpenState(preferences.open))
    const targetWidth = preferences.open ? preferences.width : 0
    sidebarResizePersistDisabledRef.current = true
    sidebarResizeWidthCaptureDisabledRef.current = true
    sidebarPanelRef.current?.resize(targetWidth)
    window.requestAnimationFrame(() => {
      sidebarResizePersistDisabledRef.current = false
      sidebarResizeWidthCaptureDisabledRef.current = false
    })
  }, [authLoading, sidebarPanelRef, user?.id])

  useEffect(
    () => () => {
      if (sidebarPreferencesPersistTimer.current) {
        clearTimeout(sidebarPreferencesPersistTimer.current)
      }
    },
    []
  )

  const toggleSidebar = useCallback(() => {
    const isOpen = sidebarOpenRef.current
    resizeSidebar(isOpen ? 0 : sidebarWidthRef.current, { persist: true })
  }, [resizeSidebar])

  const setRightSize = useCallback(
    (percent: number) => {
      animatePanel(rightPanelRef.current, rightAnim, percent, "%")
    },
    [animatePanel, rightPanelRef]
  )

  const beginRightManualResize = useCallback(() => {
    rightManualResizeRef.current = true
    rightAnim.current?.stop()
  }, [])

  const commitRightManualResize = useCallback(() => {
    if (!rightManualResizeRef.current) return
    rightManualResizeRef.current = false
    const size = rightPanelRef.current?.getSize()
    if (!size) return

    const sessionKey = workspaceSessionKeyRef.current
    const open = size.inPixels > RIGHT_PANEL_COLLAPSED_THRESHOLD_PX
    const wasOpen = rightPanelOpenRef.current
    const nextPercent = size.asPercentage
    updateRightOpen(open)
    rightPanelOpenRef.current = open

    if (!open) {
      setRightSize(0)
      if (wasOpen || rightMaximized) {
        rightSkipNextSyncRef.current = true
        setRightPanelOpen(sessionKey, false)
      }
      return
    }

    const sizeChanged =
      Math.abs(nextPercent - rightPanelSizePercentRef.current) >
      RIGHT_PANEL_SIZE_EPSILON
    rightPanelSizePercentRef.current = nextPercent
    if (rightMaximized) {
      rightSkipNextSyncRef.current = true
      setRightPanelMaximized(sessionKey, false)
    }
    if (sizeChanged || rightMaximized) {
      setRightPanelSize(sessionKey, nextPercent)
    }
    if (!wasOpen) {
      rightSkipNextSyncRef.current = true
      setRightPanelOpen(sessionKey, true)
    }
  }, [
    rightMaximized,
    rightPanelRef,
    setRightPanelMaximized,
    setRightPanelOpen,
    setRightPanelSize,
    setRightSize,
    updateRightOpen,
  ])

  const beginRightKeyboardResize = useCallback(
    (event: KeyboardEvent) => {
      if (
        event.key === "ArrowLeft" ||
        event.key === "ArrowRight" ||
        event.key === "Home" ||
        event.key === "End" ||
        event.key === "Enter"
      ) {
        beginRightManualResize()
      }
    },
    [beginRightManualResize]
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
        resizeSidebar(0, { persist: false })
      }
    } else if (sidebarWasOpenRef.current && !sidebarOpenRef.current) {
      resizeSidebar(sidebarWidthRef.current, { persist: false })
    }
  }, [
    rightMaximized,
    resizeSidebar,
    setRightPanelOpen,
    setRightPanelMaximized,
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
    setRoutePreviewSession(null)
    router.push("/w")
  }, [router])

  const openChannel = useCallback(
    (channelSlug: string) => {
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

  const runtimeAccessNeeded =
    Boolean(routeSessionID) &&
    !routeIsTemporarySession &&
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
    runtimeAccessNeeded && !sandboxRuntimeReady && !sandboxRuntimeError
  const sandboxAccessPendingForSession = sandboxRuntimePending

  const loadSandboxAccess = useCallback(
    (options: { force?: boolean } = {}) => {
      if (!routeSessionID || routeIsTemporarySession) return
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
          const access = await getSessionSandboxAccess(
            routeSessionID,
            queryClient,
            {
              expectedSandboxId: routeSandboxID,
              force: options.force,
            }
          )
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
    },
    [queryClient, routeIsTemporarySession, routeSandboxID, routeSessionID]
  )

  const refreshSandboxAccess = useCallback(() => {
    loadSandboxAccess({ force: true })
  }, [loadSandboxAccess])

  useEffect(() => {
    if (!routeSessionID || routeIsTemporarySession || !runtimeAccessNeeded) {
      sandboxRuntimeRequestRef.current += 1
      if (!routeSessionID || routeIsTemporarySession) {
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
    routeIsTemporarySession,
    routeSessionID,
    runtimeAccessNeeded,
  ])

  useEffect(() => {
    hydrateSessionRuntimeFromResponse(
      routeSessionQuery.data?.session,
      queryClient
    )
  }, [queryClient, routeSessionQuery.data?.session])

  useEffect(() => {
    if (rightSkipNextSyncRef.current) {
      rightSkipNextSyncRef.current = false
      return
    }
    const nextSize = rightPanelOpen
      ? rightMaximized
        ? RIGHT_MAX_SIZE
        : rightPanelSizePercentRef.current
      : 0
    setRightSize(nextSize)
  }, [rightMaximized, rightPanelOpen, setRightSize, workspaceSessionKey])

  const workspace = useMemo(
    () => ({
      session,
      startNewChat,
      openChannel,
      openChat,
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
  const openRenameSession = useCallback((sessionID: string, name: string) => {
    if (!sessionID || sessionID.startsWith("tmp_")) return
    setRenameSession({ id: sessionID, name })
  }, [])
  const openShareSession = useCallback((sessionID: string) => {
    if (!sessionID || sessionID.startsWith("tmp_")) return
    setShareSessionID(sessionID)
  }, [])
  const { mutate: archiveSessionMutate } = $api.useMutation(
    "delete",
    "/v1/sessions/{id}"
  )
  const archiveSession = useCallback(
    (sessionID: string) => {
      if (!sessionID || sessionID.startsWith("tmp_")) return
      archiveSessionMutate(
        { params: { path: { id: sessionID } } },
        {
          onSuccess: (response) => {
            removeSessionFromChannelCache(
              queryClient,
              response.session?.channel_id,
              sessionID
            )
            invalidateSessionListQueries(queryClient)
            toast.success("Chat archived")
            if (routeSessionID === sessionID) {
              router.push("/w")
            }
          },
          onError: (error) =>
            toast.danger(extractErrorMessage(error, "Could not archive chat")),
        }
      )
    },
    [archiveSessionMutate, queryClient, routeSessionID, router]
  )

  return (
    <WorkspaceContext.Provider value={workspace}>
      <LineCommentsProvider scopeKey={routeSessionID ?? "new-chat"}>
        <div className="h-screen w-screen overflow-hidden bg-surface text-foreground">
          <Group
            orientation="horizontal"
            className="h-full w-full"
            onLayoutChanged={commitRightManualResize}
          >
            <Panel
              id="sidebar"
              panelRef={sidebarPanelRef}
              defaultSize={DEFAULT_SIDEBAR_PREFERENCES.width}
              minSize={0}
              maxSize={SIDEBAR_MAX_WIDTH}
              className="min-w-0 overflow-hidden"
              onResize={handleSidebarResize}
            >
              <div className="h-full min-w-[230px]">
                <Sidebar
                  onCollapse={toggleSidebar}
                  onRenameSession={openRenameSession}
                  onShareSession={openShareSession}
                  onArchiveSession={archiveSession}
                />
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
                  sessionId={
                    routeSessionID && !routeIsTemporarySession
                      ? routeSessionID
                      : undefined
                  }
                  sidebarOpen={sidebarOpen}
                  onExpandSidebar={toggleSidebar}
                  onRename={
                    routeSessionID && !routeIsTemporarySession && session
                      ? () => openRenameSession(routeSessionID, session.title)
                      : undefined
                  }
                  onShare={
                    routeSessionID && !routeIsTemporarySession
                      ? () => openShareSession(routeSessionID)
                      : undefined
                  }
                  onArchive={
                    routeSessionID && !routeIsTemporarySession
                      ? () => archiveSession(routeSessionID)
                      : undefined
                  }
                  rightOpen={rightPanelOpen || rightOpen}
                  onToggleRight={toggleRight}
                />
                <div className="relative min-h-0 flex-1">{children}</div>
              </div>
            </Panel>

            <Separator
              onPointerDownCapture={beginRightManualResize}
              onKeyDownCapture={beginRightKeyboardResize}
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
                const open = size.inPixels > RIGHT_PANEL_COLLAPSED_THRESHOLD_PX
                updateRightOpen(open)
              }}
            >
              <div className="h-full min-w-90">
                <RightPanel
                  sessionId={routeSessionID}
                  channelId={routeSessionQuery.data?.session?.channel_id}
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
          <RenameSessionModal
            key={
              renameSession
                ? `${renameSession.id}:${renameSession.name}`
                : "rename-session"
            }
            session={renameSession}
            open={Boolean(renameSession)}
            onOpenChange={(next) => {
              if (!next) setRenameSession(null)
            }}
          />
          <ShareSessionModal
            sessionId={shareSessionID}
            open={Boolean(shareSessionID)}
            onOpenChange={(next) => {
              if (!next) setShareSessionID(null)
            }}
          />
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
  const agentID = session.agent_id?.trim() ?? ""
  const apiAgent = agentID ? agentsByID?.get(agentID) : undefined
  return {
    title: sessionDisplayName(session),
    agentId: agentID,
    agentName: apiAgent ? agentDisplayName(apiAgent) : undefined,
    agentIcon: apiAgent ? agentIcon(apiAgent) : undefined,
    agentAvatarURL: agentAvatarURL(apiAgent),
    modelId: session.model?.trim() || agentModel(apiAgent) || "",
    agentTurnStatus: session.agent_turn_status,
    agentTurnID: session.agent_turn_id,
    agentTurnStartedAt: session.agent_turn_started_at,
    lastTurnOutcome: session.last_turn_outcome,
    source: session.source,
    sourceResourceKey: session.source_resource_key,
    loaded: true,
  }
}

function chatHeaderAgent(session: ChatSession): ChatHeaderAgent {
  return {
    name: session.agentName ?? "Agent",
    icon: session.agentIcon ?? "bot",
    avatarURL: session.agentAvatarURL,
  }
}

function panelViewNeedsRuntimeAccess(view: PanelViewID | null) {
  return view === "review" || view === "files"
}
