"use client"

import { Icon } from "@iconify/react"

export function FilesView() {
  return (
    <div className="flex h-full items-center justify-center px-6 text-center">
      <div className="flex max-w-sm flex-col items-center gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-xl border border-border bg-background">
          <Icon icon="lucide:folder" className="h-5 w-5 text-muted" />
        </div>
        <div className="text-sm font-medium">Files are not connected yet</div>
        <p className="text-sm leading-6 text-muted">
          Workspace files will appear here when the session exposes a file
          browser.
        </p>
      </div>
    </div>
  )
}
