"use client"

import { memo, useState } from "react"
import { Button, Popover } from "@heroui/react"
import { Icon } from "@iconify/react"
import type { components } from "@/lib/api/schema"
import {
  FilesRepoSelector,
  type FilesRepoSelectorProps,
} from "@/app/w/(chat)/_components/views/files"
import { ActiveView } from "./right-panel-active-view"
import { LauncherRow } from "./right-panel-launcher"

export type PanelViewID =
  | "review"
  | "terminal"
  | "browser"
  | "files"
  | "design"
  | "side-chat"

const PANEL_VIEWS: {
  id: PanelViewID
  label: string
  icon: string
  shortcut: string
}[] = [
  { id: "review", label: "Review", icon: "lucide:file-diff", shortcut: "⌃⇧G" },
  {
    id: "terminal",
    label: "Terminal",
    icon: "lucide:square-terminal",
    shortcut: "⌃`",
  },
  { id: "browser", label: "Browser", icon: "lucide:globe", shortcut: "⌘T" },
  { id: "files", label: "Files", icon: "lucide:folder", shortcut: "⌘P" },
  { id: "design", label: "Design", icon: "lucide:pen-tool", shortcut: "⌘D" },
  {
    id: "side-chat",
    label: "Subagents",
    icon: "lucide:bot",
    shortcut: "⌥⌘S",
  },
]

export type SessionSandboxAccessResponse =
  components["schemas"]["sessionSandboxAccessResponse"]

export const RightPanel = memo(function RightPanel({
  sessionId,
  sandboxAccess,
  sandboxAccessPending,
  sandboxAccessError,
  onRefreshSandboxAccess,
  openViews,
  activeView,
  maximized,
  onSelectView,
  onOpenView,
  onCloseView,
  onToggleMaximize,
  onClosePanel,
}: {
  sessionId?: string
  sandboxAccess?: SessionSandboxAccessResponse
  sandboxAccessPending: boolean
  sandboxAccessError: unknown
  onRefreshSandboxAccess: () => void
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
  const [filesHeader, setFilesHeader] = useState<FilesRepoSelectorProps | null>(
    null
  )

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
                className={`group flex min-w-0 items-center gap-1.5 rounded-lg border py-1 pr-1.5 pl-2.5 text-sm transition-colors ${
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
                {id === "files" && filesHeader ? (
                  <FilesRepoSelector {...filesHeader} />
                ) : null}
                <button
                  type="button"
                  aria-label={`Close ${view.label}`}
                  className="rounded p-0.5 opacity-0 transition-opacity group-hover:opacity-100 hover:bg-default"
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
            <Icon
              icon="lucide:panel-right-close"
              className="h-4 w-4 text-muted"
            />
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
          <ActiveView
            id={activeView}
            sessionId={sessionId}
            sandboxAccess={sandboxAccess}
            sandboxAccessPending={sandboxAccessPending}
            sandboxAccessError={sandboxAccessError}
            onRefreshSandboxAccess={onRefreshSandboxAccess}
            onFilesHeaderChange={setFilesHeader}
          />
        )}
      </div>
    </div>
  )
})
