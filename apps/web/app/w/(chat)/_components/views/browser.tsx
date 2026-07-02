"use client"

import { Button } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import {
  selectSessionWorkspace,
  useSessionWorkspaceStore,
} from "@/app/w/(chat)/_stores/session-workspace-store"
import { previewBrowserTargetFromURL } from "@/app/w/(chat)/_lib/preview-browser-links"

export function BrowserView({
  sessionId = "new-chat",
}: {
  sessionId?: string
}) {
  const browser = useSessionWorkspaceStore(
    (state) => selectSessionWorkspace(state, sessionId).browser
  )
  const setBrowserURL = useSessionWorkspaceStore((state) => state.setBrowserURL)
  const navigateBrowser = useSessionWorkspaceStore(
    (state) => state.navigateBrowser
  )
  const reloadBrowser = useSessionWorkspaceStore((state) => state.reloadBrowser)

  const navigate = () => {
    const input = browser.url.trim()
    if (!input) return
    if (input.startsWith("/")) {
      navigateBrowser(sessionId, input)
      return
    }
    const previewTarget = previewBrowserTargetFromURL(input)
    if (previewTarget) {
      navigateBrowser(sessionId, previewTarget.url)
      return
    }
    navigateBrowser(sessionId, "/")
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex shrink-0 items-center gap-1 border-b border-border px-2 py-1.5">
        <Button variant="ghost" size="sm" isIconOnly aria-label="Back">
          <AppIcon icon="arrow-left" className="h-4 w-4 text-muted" />
        </Button>
        <Button variant="ghost" size="sm" isIconOnly aria-label="Forward">
          <AppIcon icon="arrow-right" className="h-4 w-4 text-muted/50" />
        </Button>
        <Button
          variant="ghost"
          size="sm"
          isIconOnly
          aria-label="Reload"
          onPress={() => reloadBrowser(sessionId)}
        >
          <AppIcon icon="rotate-cw" className="h-4 w-4 text-muted" />
        </Button>

        <div className="mx-1 flex h-8 min-w-0 flex-1 items-center rounded-lg bg-default px-3">
          <input
            type="text"
            value={browser.url}
            spellCheck={false}
            aria-label="Address bar"
            onChange={(event) => setBrowserURL(sessionId, event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") navigate()
            }}
            className="w-full bg-transparent text-center text-sm outline-none"
          />
        </div>

        <Button variant="ghost" size="sm" isIconOnly aria-label="Screenshot">
          <AppIcon icon="scan" className="h-4 w-4 text-muted" />
        </Button>
        <Button variant="ghost" size="sm" isIconOnly aria-label="Annotate">
          <AppIcon icon="circle-plus" className="h-4 w-4 text-muted" />
        </Button>
        <Button variant="ghost" size="sm" isIconOnly aria-label="More">
          <AppIcon
            icon="ellipsis-vertical"
            className="h-4 w-4 text-muted"
          />
        </Button>
      </div>

      <div className="min-h-0 flex-1 bg-white">
        <iframe
          key={browser.reloadKey}
          src={browser.src}
          title="Browser preview"
          className="h-full w-full border-0"
        />
      </div>
    </div>
  )
}
