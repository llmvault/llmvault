"use client"

import { Button, ListBox, Select } from "@heroui/react"
import { HugeiconsIcon } from "@hugeicons/react"
import { Paperclip, ArrowUpRight01Icon } from "@hugeicons/core-free-icons"
import { useState } from "react"
import { cn } from "@/lib/utils"
import { ClaudeIcon } from "./claude"
import { DeepseekIcon } from "./deepseek"
import { GeminiIcon } from "./gemini"
import { GrokIcon } from "./grok"
import { MoonshotIcon } from "./moonshot"
import { QwenIcon } from "./qwen"
import { XaiIcon } from "./xai"

const models = [
  { id: "claude", label: "Claude", Icon: ClaudeIcon },
  { id: "deepseek", label: "DeepSeek", Icon: DeepseekIcon },
  { id: "gemini", label: "Gemini", Icon: GeminiIcon },
  { id: "grok", label: "Grok", Icon: GrokIcon },
  { id: "moonshot", label: "Moonshot", Icon: MoonshotIcon },
  { id: "qwen", label: "Qwen", Icon: QwenIcon },
  { id: "xai", label: "xAI", Icon: XaiIcon },
]

interface ChatInputProps {
  channelName?: string
  value?: string
  onValueChange?: (value: string) => void
  onSubmit?: (value: string) => void
  disabled?: boolean
  isSending?: boolean
  placeholder?: string
  compact?: boolean
  className?: string
}

export function ChatInput({
  channelName: _channelName,
  value,
  onValueChange,
  onSubmit,
  disabled = false,
  isSending = false,
  placeholder = "Why is the production database crashing?",
  compact = false,
  className,
}: ChatInputProps) {
  const [internalValue, setInternalValue] = useState("")
  const inputValue = value ?? internalValue
  const canSubmit = inputValue.trim().length > 0 && !disabled && !isSending

  function updateValue(nextValue: string) {
    if (value === undefined) {
      setInternalValue(nextValue)
    }
    onValueChange?.(nextValue)
  }

  function submit() {
    if (!canSubmit) return
    onSubmit?.(inputValue.trim())
    if (value === undefined) {
      setInternalValue("")
    }
  }

  return (
    <div
      className={cn(
        compact ? "w-full" : "h-screen w-full flex-1 p-2",
        className
      )}
    >
      <div className="h-full w-full">
        <div className="mx-auto flex h-full w-full flex-col items-center">
          <div className="flex w-full justify-center">
            <div className="bg-surface flex min-h-36 w-full flex-col rounded-3xl shadow">
              <textarea
                value={inputValue}
                disabled={disabled || isSending}
                placeholder={placeholder}
                onChange={(event) => updateValue(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === "Enter" && !event.shiftKey) {
                    event.preventDefault()
                    submit()
                  }
                }}
                className="h-32 flex-1 resize-none border-0 bg-transparent p-4 shadow-none outline-none hover:border-0 hover:bg-transparent focus:border-0 focus:bg-transparent focus:ring-0 focus:ring-offset-0 focus:outline-none active:border-0 active:bg-transparent"
              ></textarea>
              <div className="flex w-full items-center justify-between rounded-b-3xl p-4">
                <div className="flex items-center gap-2">
                  <Button
                    isIconOnly
                    size="sm"
                    variant="secondary"
                    isDisabled={disabled || isSending}
                  >
                    <HugeiconsIcon icon={Paperclip} />
                  </Button>

                  <Select
                    variant="secondary"
                    className="min-w-36"
                    isDisabled={disabled || isSending}
                  >
                    <Select.Trigger className="flex items-center gap-2">
                      <Select.Value />
                      <Select.Indicator />
                    </Select.Trigger>
                    <Select.Popover>
                      <ListBox>
                        {models.map((model) => (
                          <ListBox.Item key={model.id} textValue={model.label}>
                            <div className="flex items-center gap-2">
                              <model.Icon className="h-4 w-4" />
                              <span>{model.label}</span>
                            </div>
                          </ListBox.Item>
                        ))}
                      </ListBox>
                    </Select.Popover>
                  </Select>
                </div>

                <Button
                  variant="primary"
                  size="sm"
                  isIconOnly
                  isDisabled={!canSubmit}
                  onPress={submit}
                >
                  <HugeiconsIcon icon={ArrowUpRight01Icon} />
                </Button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
