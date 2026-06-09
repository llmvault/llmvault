"use client"

import { Button, ListBox, Select } from "@heroui/react"
import { HugeiconsIcon } from "@hugeicons/react"
import { Paperclip, ArrowUpRight01Icon } from "@hugeicons/core-free-icons"
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
}

export function ChatInput({ channelName }: ChatInputProps) {
  return (
    <div className="h-screen w-full flex-1 p-2">
      <div className="h-full w-full">
        <div className="mx-auto flex h-full w-full max-w-4xl flex-col items-center pt-48">
          {channelName && (
            <div className="mb-4 flex items-center gap-2">
              <span className="text-lg font-medium">#{channelName}</span>
            </div>
          )}
          <div className="flex w-full max-w-2xl justify-center p-4">
            <div className="bg-surface flex min-h-36 w-full flex-col rounded-3xl shadow">
              <textarea
                placeholder="Why is the production database crashing?"
                className="h-32 flex-1 resize-none border-0 bg-transparent p-4 shadow-none outline-none hover:border-0 hover:bg-transparent focus:border-0 focus:bg-transparent focus:ring-0 focus:ring-offset-0 focus:outline-none active:border-0 active:bg-transparent"
              ></textarea>
              <div className="flex w-full items-center justify-between rounded-b-3xl p-4">
                <div className="flex items-center gap-2">
                  <Button isIconOnly size="sm" variant="secondary">
                    <HugeiconsIcon icon={Paperclip} />
                  </Button>

                  <Select variant="secondary" className="min-w-36">
                    <Select.Trigger className="flex items-center gap-2">
                      <Select.Value />
                      <Select.Indicator />
                    </Select.Trigger>
                    <Select.Popover>
                      <ListBox>
                        {models.map((model) => (
                          <ListBox.Item
                            key={model.id}
                            textValue={model.label}
                          >
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

                <Button variant="primary" size="sm" isIconOnly>
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
