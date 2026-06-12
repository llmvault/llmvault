"use client"

import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useRef,
  useState,
} from "react"
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
import { Sidebar } from "./sidebar"
import { RightPanel, type PanelViewID } from "./right-panel"
import { agentById, DEFAULT_AGENT_ID, type Agent } from "../_lib/agents"
import { presentCollaborators } from "../_lib/static-data"

const SIDEBAR_WIDTH = 300
const RIGHT_SIZE = 42 // percent
const RIGHT_MAX_SIZE = 70 // percent
const PANEL_EASE = [0.32, 0.72, 0, 1] as const

// A chat session is pinned to one agent for its whole lifetime; only the
// model can change after creation, and only within the agent's model list.
export interface ChatSession {
  title: string
  agentId: string
  modelId: string
  initialMessage?: string
}

interface WorkspaceContextValue {
  session: ChatSession | null
  startNewChat: () => void
  openChat: (title: string, agentId: string) => void
  startSession: (agentId: string, firstMessage: string, modelId?: string) => void
  setModel: (modelId: string) => void
  openView: (id: PanelViewID) => void
}

const WorkspaceContext = createContext<WorkspaceContextValue | null>(null)

export function useWorkspace() {
  const context = useContext(WorkspaceContext)
  if (!context) {
    throw new Error("useWorkspace must be used inside WorkspaceShell")
  }
  return context
}

export function WorkspaceShell({ children }: { children: React.ReactNode }) {
  const [sidebarOpen, setSidebarOpen] = useState(true)
  const [rightOpen, setRightOpen] = useState(false)
  const [openViews, setOpenViews] = useState<PanelViewID[]>([])
  const [activeView, setActiveView] = useState<PanelViewID | null>(null)
  const [session, setSession] = useState<ChatSession | null>({
    title: "Add Glitchtip support",
    agentId: DEFAULT_AGENT_ID,
    modelId: agentById(DEFAULT_AGENT_ID).defaultModelId,
  })
  const [rightMaximized, setRightMaximized] = useState(false)

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
        onUpdate: (value) =>
          handle.resize(unit === "px" ? value : `${value}%`),
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

  const startNewChat = useCallback(() => setSession(null), [])

  const openChat = useCallback((title: string, agentId: string) => {
    setSession({
      title,
      agentId,
      modelId: agentById(agentId).defaultModelId,
    })
  }, [])

  const startSession = useCallback(
    (agentId: string, firstMessage: string, modelId?: string) => {
      const agent = agentById(agentId)
      const title =
        firstMessage.length > 44
          ? `${firstMessage.slice(0, 44).trimEnd()}…`
          : firstMessage
      setSession({
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
    setSession((current) => {
      if (!current) return current
      if (!agentById(current.agentId).modelIds.includes(modelId)) return current
      return { ...current, modelId }
    })
  }, [])

  const workspace = useMemo(
    () => ({ session, startNewChat, openChat, startSession, setModel, openView }),
    [session, startNewChat, openChat, startSession, setModel, openView]
  )

  return (
    <WorkspaceContext.Provider value={workspace}>
      <div className="h-screen w-screen overflow-hidden bg-surface text-foreground">
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
                agent={session ? agentById(session.agentId) : null}
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

const EDITORS = [
  { id: "vscode", label: "Open in VS Code", icon: "vscode-icons:file-type-vscode" },
  { id: "cursor", label: "Open in Cursor", icon: "lucide:square-mouse-pointer" },
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
  agent: Agent | null
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
          <Icon icon={agent.icon} className="h-3.5 w-3.5" />
          {agent.name}
        </span>
      ) : null}
      <Popover isOpen={actionsOpen} onOpenChange={setActionsOpen}>
        <Popover.Trigger
          aria-label="Chat options"
          className="flex items-center rounded-lg p-1.5 text-muted transition-colors hover:bg-default"
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
                className={`flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors hover:bg-default ${
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
            className="flex items-center gap-1 rounded-lg px-2 py-1.5 transition-colors hover:bg-default"
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
                  className="flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors hover:bg-default"
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
  const [open, setOpen] = useState(false)
  const shown = presentCollaborators.slice(0, 3)
  const extra = presentCollaborators.length - shown.length

  return (
    <Popover isOpen={open} onOpenChange={setOpen}>
      <Popover.Trigger
        aria-label={`${presentCollaborators.length} people viewing`}
        className="mr-1 flex items-center gap-1.5 rounded-full py-0.5 pl-0.5 pr-2 transition-colors hover:bg-default"
      >
        <span className="flex items-center -space-x-1.5">
          {shown.map((person) => (
            <span
              key={person.id}
              className="flex h-6 w-6 items-center justify-center rounded-full text-[11px] font-semibold text-white ring-2 ring-surface"
              style={{ backgroundColor: person.color }}
            >
              {person.initials}
            </span>
          ))}
          {extra > 0 ? (
            <span className="flex h-6 w-6 items-center justify-center rounded-full bg-default text-[11px] font-semibold text-muted ring-2 ring-surface">
              +{extra}
            </span>
          ) : null}
        </span>
      </Popover.Trigger>
      <Popover.Content className="w-56 rounded-2xl border border-border p-1.5">
        <Popover.Dialog className="flex w-full flex-col gap-0.5 p-0">
          <span className="px-2.5 pb-1 pt-1.5 text-xs text-muted">
            {presentCollaborators.length} viewing
          </span>
          {presentCollaborators.map((person) => (
            <div
              key={person.id}
              className="flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-sm"
            >
              <span
                className="flex h-6 w-6 items-center justify-center rounded-full text-[11px] font-semibold text-white"
                style={{ backgroundColor: person.color }}
              >
                {person.initials}
              </span>
              <span className="min-w-0 flex-1 truncate">{person.name}</span>
              <span className="h-1.5 w-1.5 rounded-full bg-success" />
            </div>
          ))}
        </Popover.Dialog>
      </Popover.Content>
    </Popover>
  )
}
