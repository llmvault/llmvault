"use client"

import { useState } from "react"
import { Button, Popover } from "@heroui/react"
import { Icon } from "@iconify/react"
import { ReviewView } from "./views/review"
import { TerminalView } from "./views/terminal"
import { BrowserView } from "./views/browser"
import { FilesView } from "./views/files"
import { SideChatView } from "./views/side-chat"

export type PanelViewID = "review" | "terminal" | "browser" | "files" | "side-chat"

const PANEL_VIEWS: {
  id: PanelViewID
  label: string
  icon: string
  shortcut: string
}[] = [
  { id: "review", label: "Review", icon: "lucide:file-diff", shortcut: "⌃⇧G" },
  { id: "terminal", label: "Terminal", icon: "lucide:square-terminal", shortcut: "⌃`" },
  { id: "browser", label: "Browser", icon: "lucide:globe", shortcut: "⌘T" },
  { id: "files", label: "Files", icon: "lucide:folder", shortcut: "⌘P" },
  { id: "side-chat", label: "Side chat", icon: "lucide:message-square-plus", shortcut: "⌥⌘S" },
]

export function RightPanel({
  openViews,
  activeView,
  maximized,
  onSelectView,
  onOpenView,
  onCloseView,
  onToggleMaximize,
  onClosePanel,
}: {
  openViews: PanelViewID[]
  activeView: PanelViewID | null
  maximized: boolean
  onSelectView: (id: PanelViewID) => void
  onOpenView: (id: PanelViewID) => void
  onCloseView: (id: PanelViewID) => void
  onToggleMaximize: () => void
  onClosePanel: () => void
}) {
  const [addMenuOpen, setAddMenuOpen] = useState(false)
  const unopened = PANEL_VIEWS.filter((view) => !openViews.includes(view.id))

  return (
    <div className="flex h-full min-w-0 flex-col bg-surface">
      <div className="flex h-12 shrink-0 items-center gap-1 px-2">
        <div className="flex min-w-0 flex-1 items-center gap-1">
          {openViews.map((id) => {
            const view = PANEL_VIEWS.find((entry) => entry.id === id)
            if (!view) return null
            const isActive = id === activeView
            return (
              <div
                key={id}
                className={`group flex min-w-0 items-center gap-1.5 rounded-lg border py-1 pl-2.5 pr-1.5 text-sm transition-colors ${
                  isActive
                    ? "border-border bg-surface"
                    : "border-transparent text-muted hover:bg-default"
                }`}
              >
                <button
                  type="button"
                  className="flex min-w-0 items-center gap-1.5"
                  onClick={() => onSelectView(id)}
                >
                  <Icon icon={view.icon} className="h-3.5 w-3.5 shrink-0" />
                  <span className="truncate">{view.label}</span>
                </button>
                <button
                  type="button"
                  aria-label={`Close ${view.label}`}
                  className="rounded p-0.5 opacity-0 transition-opacity hover:bg-default group-hover:opacity-100"
                  onClick={() => onCloseView(id)}
                >
                  <Icon icon="lucide:x" className="h-3 w-3" />
                </button>
              </div>
            )
          })}

          {unopened.length > 0 ? (
            <Popover isOpen={addMenuOpen} onOpenChange={setAddMenuOpen}>
              <Popover.Trigger
                aria-label="Open view"
                className="flex items-center rounded-lg p-1.5 text-muted transition-colors hover:bg-default"
              >
                <Icon icon="lucide:plus" className="h-4 w-4" />
              </Popover.Trigger>
              <Popover.Content className="w-64 rounded-2xl border border-border p-1.5">
                <Popover.Dialog className="flex w-full flex-col gap-0.5 p-0">
                  {unopened.map((view) => (
                    <LauncherRow
                      key={view.id}
                      icon={view.icon}
                      label={view.label}
                      shortcut={view.shortcut}
                      compact
                      onPress={() => {
                        setAddMenuOpen(false)
                        onOpenView(view.id)
                      }}
                    />
                  ))}
                </Popover.Dialog>
              </Popover.Content>
            </Popover>
          ) : null}
        </div>

        <div className="flex shrink-0 items-center gap-0.5">
          <Button
            variant="ghost"
            size="sm"
            isIconOnly
            aria-label={maximized ? "Restore panel" : "Expand panel"}
            onPress={onToggleMaximize}
          >
            <Icon
              icon={maximized ? "lucide:minimize-2" : "lucide:maximize-2"}
              className="h-4 w-4 text-muted"
            />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            isIconOnly
            aria-label="Close panel"
            onPress={onClosePanel}
          >
            <Icon icon="lucide:panel-right-close" className="h-4 w-4 text-muted" />
          </Button>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-hidden">
        {activeView === null ? (
          <div className="flex h-full items-end px-4 pb-24">
            <div className="flex w-full flex-col gap-2">
              {PANEL_VIEWS.map((view) => (
                <LauncherRow
                  key={view.id}
                  icon={view.icon}
                  label={view.label}
                  shortcut={view.shortcut}
                  onPress={() => onOpenView(view.id)}
                />
              ))}
            </div>
          </div>
        ) : (
          <ActiveView id={activeView} />
        )}
      </div>
    </div>
  )
}

function LauncherRow({
  icon,
  label,
  shortcut,
  compact,
  onPress,
}: {
  icon: string
  label: string
  shortcut: string
  compact?: boolean
  onPress: () => void
}) {
  return (
    <button
      type="button"
      onClick={onPress}
      className={`flex w-full items-center gap-2.5 rounded-xl text-left text-sm transition-colors hover:bg-default ${
        compact ? "px-2.5 py-1.5" : "bg-background px-3 py-2.5"
      }`}
    >
      <Icon icon={icon} className="h-4 w-4 shrink-0 text-muted" />
      <span className="min-w-0 flex-1 truncate">{label}</span>
      <span className="shrink-0 font-sans text-xs text-muted">{shortcut}</span>
    </button>
  )
}

function ActiveView({ id }: { id: PanelViewID }) {
  switch (id) {
    case "review":
      return <ReviewView />
    case "terminal":
      return <TerminalView />
    case "browser":
      return <BrowserView />
    case "files":
      return <FilesView />
    case "side-chat":
      return <SideChatView />
  }
}
