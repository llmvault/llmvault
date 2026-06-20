"use client"

import { useState } from "react"
import { Button, Popover } from "@heroui/react"
import { Icon } from "@iconify/react"
import { ChatHeaderAgentLogo } from "./chat-header-agent-logo"
import type { ChatHeaderAgent } from "./chat-header-types"

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

export function ChatHeader({
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
