"use client"

import { useRef, useState } from "react"
import { Button, Popover } from "@heroui/react"
import { Icon } from "@iconify/react"
import { modelById, type Agent } from "../_lib/agents"

const ACCESS_MODES = [
  {
    id: "full",
    label: "Full access",
    icon: "lucide:octagon-alert",
    description: "Edit files, run commands, and use the network",
    warning: true,
  },
  {
    id: "edits",
    label: "Approve edits",
    icon: "lucide:file-check-2",
    description: "Ask before changing files or running commands",
    warning: false,
  },
  {
    id: "read",
    label: "Read only",
    icon: "lucide:eye",
    description: "Explore the workspace without making changes",
    warning: false,
  },
]

const EFFORTS = ["Low", "Medium", "High"]

export function Composer({
  agent,
  modelId,
  onModelChange,
  onSend,
  placeholder = "Ask for follow-up changes",
  isStreaming = false,
  onStop,
}: {
  agent: Agent
  modelId: string
  onModelChange: (modelId: string) => void
  onSend: (text: string) => void
  placeholder?: string
  isStreaming?: boolean
  onStop?: () => void
}) {
  const [value, setValue] = useState("")
  const [accessMode, setAccessMode] = useState(ACCESS_MODES[0])
  const [effort, setEffort] = useState("High")
  const [accessOpen, setAccessOpen] = useState(false)
  const [modelOpen, setModelOpen] = useState(false)
  const [recording, setRecording] = useState(false)
  const textareaRef = useRef<HTMLTextAreaElement | null>(null)

  const selectedModel = modelById(modelId)
  const canSend = value.trim().length > 0

  const submit = () => {
    if (!canSend) return
    onSend(value.trim())
    setValue("")
    const node = textareaRef.current
    if (node) node.style.height = "auto"
  }

  return (
    <div className="mx-auto w-full max-w-3xl px-4 pb-4">
      <div className="flex flex-col gap-2 rounded-3xl border border-border bg-surface px-3 pb-2 pt-3 shadow-sm">
        <textarea
          ref={textareaRef}
          rows={1}
          value={value}
          placeholder={placeholder}
          onChange={(event) => {
            setValue(event.target.value)
            event.target.style.height = "auto"
            event.target.style.height = `${Math.min(event.target.scrollHeight, 160)}px`
          }}
          onKeyDown={(event) => {
            if (event.key === "Enter" && !event.shiftKey) {
              event.preventDefault()
              submit()
            }
          }}
          className="max-h-40 w-full resize-none bg-transparent px-2 text-[15px] outline-none placeholder:text-muted"
        />

        <div className="flex items-center gap-1">
          <Button variant="ghost" size="sm" isIconOnly aria-label="Attach">
            <Icon icon="lucide:plus" className="h-4 w-4 text-muted" />
          </Button>

          <Popover isOpen={accessOpen} onOpenChange={setAccessOpen}>
            <Popover.Trigger
              aria-label="Access mode"
              className={`flex items-center gap-1.5 rounded-lg px-2 py-1.5 text-sm transition-colors hover:bg-default ${
                accessMode.warning ? "text-warning" : "text-muted"
              }`}
            >
              <Icon icon={accessMode.icon} className="h-4 w-4" />
              {accessMode.label}
              <Icon icon="lucide:chevron-down" className="h-3.5 w-3.5" />
            </Popover.Trigger>
            <Popover.Content className="w-72 rounded-2xl border border-border p-1.5">
              <Popover.Dialog className="flex w-full flex-col gap-0.5 p-0">
                {ACCESS_MODES.map((mode) => (
                  <button
                    key={mode.id}
                    type="button"
                    onClick={() => {
                      setAccessMode(mode)
                      setAccessOpen(false)
                    }}
                    className="flex items-start gap-2.5 rounded-xl px-2.5 py-2 text-left transition-colors hover:bg-default"
                  >
                    <Icon
                      icon={mode.icon}
                      className={`mt-0.5 h-4 w-4 shrink-0 ${mode.warning ? "text-warning" : "text-muted"}`}
                    />
                    <span className="flex min-w-0 flex-1 flex-col">
                      <span className="text-sm">{mode.label}</span>
                      <span className="text-xs text-muted">
                        {mode.description}
                      </span>
                    </span>
                    {mode.id === accessMode.id ? (
                      <Icon
                        icon="lucide:check"
                        className="mt-1 h-4 w-4 shrink-0"
                      />
                    ) : null}
                  </button>
                ))}
              </Popover.Dialog>
            </Popover.Content>
          </Popover>

          <div className="flex-1" />

          <Popover isOpen={modelOpen} onOpenChange={setModelOpen}>
            <Popover.Trigger
              aria-label="Model and effort"
              className="flex items-center gap-1.5 rounded-lg px-2 py-1.5 text-sm transition-colors hover:bg-default"
            >
              <selectedModel.Icon className="h-3.5 w-3.5" />
              <span className="font-medium">{selectedModel.label}</span>
              <span className="text-muted">{effort}</span>
              <Icon
                icon="lucide:chevron-down"
                className="h-3.5 w-3.5 text-muted"
              />
            </Popover.Trigger>
            <Popover.Content className="w-64 rounded-2xl border border-border p-1.5">
              <Popover.Dialog className="flex w-full flex-col gap-0.5 p-0">
                <span className="px-2.5 pb-1 pt-1.5 text-xs text-muted">
                  Models available to {agent.name}
                </span>
                {agent.modelIds.map(modelById).map((entry) => (
                  <button
                    key={entry.id}
                    type="button"
                    onClick={() => onModelChange(entry.id)}
                    className="flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors hover:bg-default"
                  >
                    <entry.Icon className="h-4 w-4 shrink-0" />
                    <span className="flex min-w-0 flex-1 flex-col">
                      <span>{entry.label}</span>
                      <span className="text-xs text-muted">
                        {entry.provider}
                      </span>
                    </span>
                    {entry.id === modelId ? (
                      <Icon icon="lucide:check" className="h-4 w-4 shrink-0" />
                    ) : null}
                  </button>
                ))}
                <span className="px-2.5 pb-1 pt-2 text-xs text-muted">
                  Reasoning effort
                </span>
                {EFFORTS.map((entry) => (
                  <button
                    key={entry}
                    type="button"
                    onClick={() => {
                      setEffort(entry)
                      setModelOpen(false)
                    }}
                    className="flex items-center gap-2 rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors hover:bg-default"
                  >
                    <span className="min-w-0 flex-1">{entry}</span>
                    {entry === effort ? (
                      <Icon icon="lucide:check" className="h-4 w-4 shrink-0" />
                    ) : null}
                  </button>
                ))}
              </Popover.Dialog>
            </Popover.Content>
          </Popover>

          <Button
            variant="ghost"
            size="sm"
            isIconOnly
            aria-label={recording ? "Stop dictation" : "Dictate"}
            onPress={() => setRecording((value) => !value)}
          >
            <Icon
              icon="lucide:mic"
              className={`h-4 w-4 ${recording ? "text-danger" : "text-muted"}`}
            />
          </Button>
          {isStreaming ? (
            <Button
              variant="primary"
              size="sm"
              isIconOnly
              aria-label="Stop"
              onPress={onStop}
              className="rounded-full"
            >
              <Icon icon="lucide:square" className="h-3.5 w-3.5" />
            </Button>
          ) : (
            <Button
              variant="primary"
              size="sm"
              isIconOnly
              aria-label="Send"
              isDisabled={!canSend}
              onPress={submit}
              className="rounded-full"
            >
              <Icon icon="lucide:arrow-up" className="h-4 w-4" />
            </Button>
          )}
        </div>
      </div>
    </div>
  )
}
