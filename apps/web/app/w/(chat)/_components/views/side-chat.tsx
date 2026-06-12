"use client"

import { useState } from "react"
import { Button } from "@heroui/react"
import { Icon } from "@iconify/react"
import { sideChatMessages } from "../../_lib/static-data"
import { InlineProse } from "../conversation"

const CANNED_REPLY =
  "Good question — in this static exploration I can only point you back at the main thread, but the layout you're looking at is the real side-chat design."

export function SideChatView() {
  const [messages, setMessages] = useState(sideChatMessages)
  const [value, setValue] = useState("")

  const send = () => {
    const text = value.trim()
    if (!text) return
    setMessages((current) => [
      ...current,
      { role: "user", text },
      { role: "assistant", text: CANNED_REPLY },
    ])
    setValue("")
  }

  return (
    <div className="flex h-full flex-col">
      <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4">
        <div className="flex flex-col gap-4">
          {messages.map((message, index) =>
            message.role === "user" ? (
              <div
                key={index}
                className="max-w-[85%] self-end rounded-2xl bg-default px-3.5 py-2 text-sm"
              >
                {message.text}
              </div>
            ) : (
              <div key={index} className="text-sm leading-6">
                <InlineProse text={message.text} />
              </div>
            )
          )}
        </div>
      </div>

      <div className="shrink-0 px-3 pb-3">
        <div className="flex items-center gap-1 rounded-2xl border border-border bg-surface px-2 py-1.5 shadow-sm">
          <Button variant="ghost" size="sm" isIconOnly aria-label="Attach">
            <Icon icon="lucide:plus" className="h-4 w-4 text-muted" />
          </Button>
          <input
            type="text"
            value={value}
            placeholder="Ask a side question"
            aria-label="Side chat input"
            onChange={(event) => setValue(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") send()
            }}
            className="min-w-0 flex-1 bg-transparent text-sm outline-none placeholder:text-muted"
          />
          <Button
            variant="primary"
            size="sm"
            isIconOnly
            aria-label="Send"
            isDisabled={!value.trim()}
            onPress={send}
            className="rounded-full"
          >
            <Icon icon="lucide:arrow-up" className="h-4 w-4" />
          </Button>
        </div>
      </div>
    </div>
  )
}
