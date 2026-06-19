"use client"

import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useRef,
  useState,
} from "react"
import { usePathname, useRouter } from "next/navigation"
import {
  Group,
  Panel,
  Separator,
  usePanelRef,
  type PanelImperativeHandle,
} from "react-resizable-panels"
import { animate, type AnimationPlaybackControls } from "motion/react"
import { Button, Popover } from "@heroui/react"
import { Icon } from "@iconify/react"
import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"
import {
  RightPanel,
  type PanelViewID,
} from "@/app/w/(chat)/_components/right-panel"
import { Sidebar } from "@/app/w/(chat)/_components/sidebar"
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
}

const WorkspaceContext = createContext<WorkspaceContextValue | null>(null)
type SessionResponse = components["schemas"]["sessionResponse"]
type AgentResponse = components["schemas"]["agentListItem"]

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
  const [sidebarOpen, setSidebarOpen] = useState(true)
  const [rightOpen, setRightOpen] = useState(false)
  const [openViews, setOpenViews] = useState<PanelViewID[]>([])
  const [activeView, setActiveView] = useState<PanelViewID | null>(null)
  const [draftSession, setDraftSession] = useState<ChatSession | null>(null)
  const [routePreviewSession, setRoutePreviewSession] = useState<{
    sessionId: string
    session: ChatSession
  } | null>(null)
  const [rightMaximized, setRightMaximized] = useState(false)

  const routeParams = useMemo(
    () => sessionRouteFromPathname(pathname),
    [pathname]
  )
  const routeSessionID = routeParams?.sessionId
  const routeIsOptimistic = routeSessionID?.startsWith("tmp_") ?? false
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

  const sidebarPanelRef = usePanelRef()
  const rightPanelRef = usePanelRef()
  const sidebarAnim = useRef<AnimationPlaybackControls | null>(null)
  const rightAnim = useRef<AnimationPlaybackControls | null>(null)
  const sidebarWasOpenRef = useRef(true)

  const animatePanel = useCallback(
    (
      handle: PanelImperativeHandle | null,
      anim: React.RefObject<AnimationPlaybackControls | null>,
      to: number,
      unit: "px" | "%"
    ) => {
      if (!handle) return
      anim.current?.stop()
      const size = handle.getSize()
      const from = unit === "px" ? size.inPixels : size.asPercentage
      anim.current = animate(from, to, {
        duration: 0.3,
        ease: PANEL_EASE,
        onUpdate: (value) => handle.resize(unit === "px" ? value : `${value}%`),
      })
    },
    []
  )

  const toggleSidebar = useCallback(() => {
    animatePanel(
      sidebarPanelRef.current,
      sidebarAnim,
      sidebarOpen ? 0 : SIDEBAR_WIDTH,
      "px"
    )
  }, [animatePanel, sidebarOpen, sidebarPanelRef])

  const setRightSize = useCallback(
    (percent: number) => {
      animatePanel(rightPanelRef.current, rightAnim, percent, "%")
    },
    [animatePanel, rightPanelRef]
  )

  const toggleRight = useCallback(() => {
    setRightMaximized(false)
    setRightSize(rightOpen ? 0 : RIGHT_SIZE)
  }, [rightOpen, setRightSize])

  const toggleMaximize = useCallback(() => {
    const next = !rightMaximized
    setRightMaximized(next)
    if (next) {
      // Maximizing the right panel takes the sidebar's room; remember
      // whether it was open so restoring brings it back.
      sidebarWasOpenRef.current = sidebarOpen
      if (sidebarOpen) {
        animatePanel(sidebarPanelRef.current, sidebarAnim, 0, "px")
      }
    } else if (sidebarWasOpenRef.current && !sidebarOpen) {
      animatePanel(sidebarPanelRef.current, sidebarAnim, SIDEBAR_WIDTH, "px")
    }
    setRightSize(next ? RIGHT_MAX_SIZE : RIGHT_SIZE)
  }, [animatePanel, rightMaximized, setRightSize, sidebarOpen, sidebarPanelRef])

  const openView = useCallback(
    (id: PanelViewID) => {
      setOpenViews((views) => (views.includes(id) ? views : [...views, id]))
      setActiveView(id)
      if (!rightOpen) setRightSize(RIGHT_SIZE)
    },
    [rightOpen, setRightSize]
  )

  const closeView = (id: PanelViewID) => {
    setOpenViews((views) => {
      const next = views.filter((view) => view !== id)
      setActiveView((current) =>
        current === id ? (next[next.length - 1] ?? null) : current
      )
      return next
    })
  }

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

  const workspace = useMemo(
    () => ({
      session,
      startNewChat,
      openChannel,
      openChat,
      startSession,
      setModel,
      openView,
    }),
    [
      session,
      startNewChat,
      openChannel,
      openChat,
      startSession,
      setModel,
      openView,
    ]
  )

  return (
    <WorkspaceContext.Provider value={workspace}>
      <div className="bg-surface h-screen w-screen overflow-hidden text-foreground">
        <Group orientation="horizontal" className="h-full w-full">
          <Panel
            id="sidebar"
            panelRef={sidebarPanelRef}
            defaultSize={SIDEBAR_WIDTH}
            minSize={0}
            maxSize={420}
            className="min-w-0 overflow-hidden"
            onResize={(size) => setSidebarOpen(size.inPixels > 8)}
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
                agent={session ? chatHeaderAgent(session) : null}
                sidebarOpen={sidebarOpen}
                onExpandSidebar={toggleSidebar}
                rightOpen={rightOpen}
                onToggleRight={toggleRight}
              />
              <div className="min-h-0 flex-1">{children}</div>
            </div>
          </Panel>

          <Separator
            className={`shrink-0 bg-border transition-colors hover:bg-accent data-[resizing]:bg-accent ${
              rightOpen ? "w-px" : "w-0"
            }`}
          />
          <Panel
            id="side"
            panelRef={rightPanelRef}
            defaultSize={0}
            minSize={0}
            className="min-w-0 overflow-hidden"
            onResize={(size) => setRightOpen(size.inPixels > 8)}
          >
            <div className="h-full min-w-[360px]">
              <RightPanel
                openViews={openViews}
                activeView={activeView}
                maximized={rightMaximized}
                onSelectView={setActiveView}
                onOpenView={openView}
                onCloseView={closeView}
                onToggleMaximize={toggleMaximize}
                onClosePanel={toggleRight}
              />
            </div>
          </Panel>
        </Group>
      </div>
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

type ChatHeaderAgent = Pick<Agent, "name" | "icon"> & {
  avatarURL?: string
}

function chatHeaderAgent(session: ChatSession): ChatHeaderAgent {
  const fallback = safeAgentById(session.agentId)
  return {
    name: session.agentName ?? fallback.name,
    icon: session.agentIcon ?? fallback.icon,
    avatarURL: session.agentAvatarURL,
  }
}

const EDITORS = [
  {
    id: "vscode",
    label: "Open in VS Code",
    icon: "vscode-icons:file-type-vscode",
  },
  {
    id: "cursor",
    label: "Open in Cursor",
    icon: "lucide:square-mouse-pointer",
  },
  { id: "zed", label: "Open in Zed", icon: "lucide:zap" },
  { id: "copy", label: "Copy worktree path", icon: "lucide:copy" },
]

const CHAT_ACTIONS = [
  { id: "rename", label: "Rename chat", icon: "lucide:pencil" },
  { id: "share", label: "Share", icon: "lucide:share" },
  { id: "move", label: "Move to project", icon: "lucide:folder-input" },
  { id: "archive", label: "Archive", icon: "lucide:archive" },
  { id: "delete", label: "Delete", icon: "lucide:trash-2", danger: true },
]

function ChatHeader({
  title,
  agent,
  sidebarOpen,
  onExpandSidebar,
  rightOpen,
  onToggleRight,
}: {
  title: string
  agent: ChatHeaderAgent | null
  sidebarOpen: boolean
  onExpandSidebar: () => void
  rightOpen: boolean
  onToggleRight: () => void
}) {
  const [actionsOpen, setActionsOpen] = useState(false)
  const [editorOpen, setEditorOpen] = useState(false)

  return (
    <div className="flex h-12 shrink-0 items-center gap-1 px-3">
      {!sidebarOpen ? (
        <Button
          variant="ghost"
          size="sm"
          isIconOnly
          aria-label="Expand sidebar"
          onPress={onExpandSidebar}
        >
          <Icon icon="lucide:panel-left-open" className="h-4 w-4" />
        </Button>
      ) : null}

      <span className="truncate px-1 text-sm font-medium">{title}</span>
      {agent ? (
        // The agent is fixed once a session exists, so this is a read-only
        // chip rather than a switcher.
        <span
          title="The agent can't be changed after a session starts"
          className="flex shrink-0 cursor-default items-center gap-1.5 rounded-full border border-border px-2 py-0.5 text-xs text-muted"
        >
          <ChatHeaderAgentLogo agent={agent} />
          {agent.name}
        </span>
      ) : null}
      <Popover isOpen={actionsOpen} onOpenChange={setActionsOpen}>
        <Popover.Trigger
          aria-label="Chat options"
          className="hover:bg-default flex items-center rounded-lg p-1.5 text-muted transition-colors"
        >
          <Icon icon="lucide:ellipsis" className="h-4 w-4" />
        </Popover.Trigger>
        <Popover.Content className="w-52 rounded-2xl border border-border p-1.5">
          <Popover.Dialog className="flex w-full flex-col gap-0.5 p-0">
            {CHAT_ACTIONS.map((action) => (
              <button
                key={action.id}
                type="button"
                onClick={() => setActionsOpen(false)}
                className={`hover:bg-default flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors ${
                  action.danger ? "text-danger" : ""
                }`}
              >
                <Icon icon={action.icon} className="h-4 w-4 shrink-0" />
                {action.label}
              </button>
            ))}
          </Popover.Dialog>
        </Popover.Content>
      </Popover>

      <div className="flex-1" />

      <PresenceStack />

      <div className="flex items-center gap-0.5">
        <Popover isOpen={editorOpen} onOpenChange={setEditorOpen}>
          <Popover.Trigger
            aria-label="Open in editor"
            className="hover:bg-default flex items-center gap-1 rounded-lg px-2 py-1.5 transition-colors"
          >
            <Icon icon="vscode-icons:file-type-vscode" className="h-4 w-4" />
            <Icon icon="lucide:chevron-down" className="h-3 w-3 text-muted" />
          </Popover.Trigger>
          <Popover.Content className="w-56 rounded-2xl border border-border p-1.5">
            <Popover.Dialog className="flex w-full flex-col gap-0.5 p-0">
              {EDITORS.map((editor) => (
                <button
                  key={editor.id}
                  type="button"
                  onClick={() => setEditorOpen(false)}
                  className="hover:bg-default flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors"
                >
                  <Icon icon={editor.icon} className="h-4 w-4 shrink-0" />
                  {editor.label}
                </button>
              ))}
            </Popover.Dialog>
          </Popover.Content>
        </Popover>
        <Button variant="ghost" size="sm" isIconOnly aria-label="Tasks">
          <Icon icon="lucide:list-todo" className="h-4 w-4 text-muted" />
        </Button>
        <Button
          variant="ghost"
          size="sm"
          isIconOnly
          aria-label="Toggle side panel"
          onPress={onToggleRight}
        >
          <Icon
            icon={rightOpen ? "lucide:panel-right-close" : "lucide:panel-right"}
            className="h-4 w-4 text-muted"
          />
        </Button>
      </div>
    </div>
  )
}

function PresenceStack() {
  return null
}

function ChatHeaderAgentLogo({ agent }: { agent: ChatHeaderAgent }) {
  const [failed, setFailed] = useState(false)
  if (agent.avatarURL && !failed) {
    return (
      <span className="bg-default flex h-4 w-4 shrink-0 items-center justify-center overflow-hidden rounded-[5px] ring-1 ring-border/70">
        <img
          src={agent.avatarURL}
          alt=""
          className="h-full w-full object-cover"
          onError={() => setFailed(true)}
        />
      </span>
    )
  }

  return <Icon icon={agent.icon} className="h-3.5 w-3.5 shrink-0" />
}
