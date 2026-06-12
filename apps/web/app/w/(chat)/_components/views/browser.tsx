"use client"

import { useState } from "react"
import { Button } from "@heroui/react"
import { Icon } from "@iconify/react"

export function BrowserView() {
  const [url, setUrl] = useState("usehivy.com")
  const [src, setSrc] = useState("/")
  const [reloadKey, setReloadKey] = useState(0)

  const navigate = () => {
    const input = url.trim()
    if (!input) return
    // Same-origin paths load directly; anything else goes through the
    // local marketing site since external sites may block embedding.
    setSrc(input.startsWith("/") ? input : "/")
    setReloadKey((key) => key + 1)
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex shrink-0 items-center gap-1 border-b border-border px-2 py-1.5">
        <Button variant="ghost" size="sm" isIconOnly aria-label="Back">
          <Icon icon="lucide:arrow-left" className="h-4 w-4 text-muted" />
        </Button>
        <Button variant="ghost" size="sm" isIconOnly aria-label="Forward">
          <Icon icon="lucide:arrow-right" className="h-4 w-4 text-muted/50" />
        </Button>
        <Button
          variant="ghost"
          size="sm"
          isIconOnly
          aria-label="Reload"
          onPress={() => setReloadKey((key) => key + 1)}
        >
          <Icon icon="lucide:rotate-cw" className="h-4 w-4 text-muted" />
        </Button>

        <div className="mx-1 flex h-8 min-w-0 flex-1 items-center rounded-lg bg-default px-3">
          <input
            type="text"
            value={url}
            spellCheck={false}
            aria-label="Address bar"
            onChange={(event) => setUrl(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") navigate()
            }}
            className="w-full bg-transparent text-center text-sm outline-none"
          />
        </div>

        <Button variant="ghost" size="sm" isIconOnly aria-label="Screenshot">
          <Icon icon="lucide:scan" className="h-4 w-4 text-muted" />
        </Button>
        <Button variant="ghost" size="sm" isIconOnly aria-label="Annotate">
          <Icon icon="lucide:circle-plus" className="h-4 w-4 text-muted" />
        </Button>
        <Button variant="ghost" size="sm" isIconOnly aria-label="More">
          <Icon icon="lucide:ellipsis-vertical" className="h-4 w-4 text-muted" />
        </Button>
      </div>

      <div className="min-h-0 flex-1 bg-white">
        <iframe
          key={reloadKey}
          src={src}
          title="Browser preview"
          className="h-full w-full border-0"
        />
      </div>
    </div>
  )
}
