"use client"

import { BrowserView } from "@/app/w/(chat)/_components/views/browser"
import { DesignView } from "@/app/w/(chat)/_components/views/design"
import {
  FilesView,
  type FilesRepoSelectorProps,
} from "@/app/w/(chat)/_components/views/files"
import { ReviewView } from "@/app/w/(chat)/_components/views/review"
import { SheetsView } from "@/app/w/(chat)/_components/views/sheets"
import { SubagentView } from "@/app/w/(chat)/_components/views/side-chat"
import type { PanelViewID, SessionSandboxAccessResponse } from "./right-panel"

export function ActiveView({
  id,
  sessionId,
  channelId,
  sandboxAccess,
  sandboxAccessPending,
  sandboxAccessError,
  onRefreshSandboxAccess,
  onFilesHeaderChange,
}: {
  id: PanelViewID
  sessionId?: string
  channelId?: string
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
    case "browser":
      return <BrowserView sessionId={sessionId} />
    case "design":
      return <DesignView sessionId={sessionId} />
    case "sheets":
      return <SheetsView channelId={channelId} />
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
      return <SubagentView sessionId={sessionId} />
  }
}
