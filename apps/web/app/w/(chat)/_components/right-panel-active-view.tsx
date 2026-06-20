"use client"

import { BrowserView } from "@/app/w/(chat)/_components/views/browser"
import {
  FilesView,
  type FilesRepoSelectorProps,
} from "@/app/w/(chat)/_components/views/files"
import { ReviewView } from "@/app/w/(chat)/_components/views/review"
import { SideChatView } from "@/app/w/(chat)/_components/views/side-chat"
import { TerminalView } from "@/app/w/(chat)/_components/views/terminal"
import type { PanelViewID, SessionSandboxAccessResponse } from "./right-panel"

export function ActiveView({
  id,
  sessionId,
  sandboxAccess,
  sandboxAccessPending,
  sandboxAccessError,
  onRefreshSandboxAccess,
  onFilesHeaderChange,
}: {
  id: PanelViewID
  sessionId?: string
  sandboxAccess?: SessionSandboxAccessResponse
  sandboxAccessPending: boolean
  sandboxAccessError: unknown
  onRefreshSandboxAccess: () => void
  onFilesHeaderChange: (props: FilesRepoSelectorProps | null) => void
}) {
  switch (id) {
    case "review":
      return (
        <ReviewView
          sessionId={sessionId}
          sandboxAccess={sandboxAccess}
          sandboxAccessPending={sandboxAccessPending}
          sandboxAccessError={sandboxAccessError}
          onRefreshSandboxAccess={onRefreshSandboxAccess}
        />
      )
    case "terminal":
      return <TerminalView />
    case "browser":
      return <BrowserView />
    case "files":
      return (
        <FilesView
          sessionId={sessionId}
          sandboxAccess={sandboxAccess}
          sandboxAccessPending={sandboxAccessPending}
          sandboxAccessError={sandboxAccessError}
          onRefreshSandboxAccess={onRefreshSandboxAccess}
          onHeaderChange={onFilesHeaderChange}
        />
      )
    case "side-chat":
      return <SideChatView />
  }
}
