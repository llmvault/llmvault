"use client"

import {
  Avatar,
  Button,
  Card,
  Chip,
  ListBox,
  Popover,
  Select,
  Separator,
  Surface,
  Typography,
} from "@heroui/react"
import { HugeiconsIcon } from "@hugeicons/react"
import { Paperclip, ArrowUpRight01Icon, PlusSignIcon } from "@hugeicons/core-free-icons"
import { Icon } from "@iconify/react"
import { ClaudeIcon } from "./_components/claude"
import { DeepseekIcon } from "./_components/deepseek"
import { GeminiIcon } from "./_components/gemini"
import { GrokIcon } from "./_components/grok"
import { MoonshotIcon } from "./_components/moonshot"
import { QwenIcon } from "./_components/qwen"
import { XaiIcon } from "./_components/xai"

const models = [
  { id: "claude", label: "Claude", Icon: ClaudeIcon },
  { id: "deepseek", label: "DeepSeek", Icon: DeepseekIcon },
  { id: "gemini", label: "Gemini", Icon: GeminiIcon },
  { id: "grok", label: "Grok", Icon: GrokIcon },
  { id: "moonshot", label: "Moonshot", Icon: MoonshotIcon },
  { id: "qwen", label: "Qwen", Icon: QwenIcon },
  { id: "xai", label: "xAI", Icon: XaiIcon },
]

const channels = [
  { id: "general", name: "general", private: false, unread: false, unreadCount: 0, active: false },
  { id: "random", name: "random", private: false, unread: true, unreadCount: 0, active: false },
  { id: "engineering", name: "engineering", private: false, unread: true, unreadCount: 3, active: true },
  { id: "design", name: "design", private: false, unread: false, unreadCount: 0, active: false },
  { id: "exec-updates", name: "exec-updates", private: true, unread: true, unreadCount: 1, active: false },
  { id: "deploys", name: "deploys", private: false, unread: false, unreadCount: 0, active: false },
]

export default function DashboardPage() {
  return (
    <div className="flex h-screen w-screen">
      <div className="h-screen w-80 p-2">
        <Card className="h-full w-full">
          <Card.Content className="flex h-full flex-col gap-4 py-3">
            <Select variant="primary" className="min-w-36">
              <Select.Trigger className="flex items-center gap-2 border border-border">
                <Select.Value />
                <Select.Indicator />
              </Select.Trigger>
              <Select.Popover>
                <ListBox>
                  {models.map((model) => (
                    <ListBox.Item key={model.id} textValue={model.label}>
                      <div className="flex items-center gap-2">
                        <div className="h-4 w-4 rounded-lg bg-amber-700" />
                        <span>{model.label}</span>
                      </div>
                    </ListBox.Item>
                  ))}
                  <ListBox.Item isDisabled>
                    <div className="flex items-center gap-2">
                      <HugeiconsIcon icon={PlusSignIcon} className="-ml-1 h-4 w-4" />
                    </div>
                    Add new workspace
                  </ListBox.Item>
                </ListBox>
              </Select.Popover>
            </Select>

            <div className="flex flex-1 flex-col gap-6 overflow-y-auto">
              <div className="flex flex-col gap-1">
                <div className="flex flex-col gap-1">
                  {channels.map((channel) =>
                    (
                      <Button
                        key={channel.id}
                        variant="ghost"
                        className={`h-auto justify-start gap-2 px-3 py-1.5 w-full ${channel?.active ? 'bg-accent' : ''}`}
                      >
                        <Icon
                          icon={channel.private ? "lucide:lock" : "lucide:hash"}
                          className={`h-4 w-4 ${channel?.active ? 'text-accent-foreground': ''}`}
                        />
                        <Typography.Paragraph
                            size="sm"
                            color="muted"
                            className={`flex-1 ${channel?.active ? 'text-accent-foreground': ''}`}
                          >
                            {channel.name}
                          </Typography.Paragraph>
                        {channel.unreadCount > 0 && (
                          <Chip size="sm" className="ml-auto">
                            {channel.unreadCount}
                          </Chip>
                        )}
                      </Button>
                    )
                  )}
                </div>
              </div>

            </div>

            <Popover>
              <Popover.Trigger className="flex cursor-pointer items-center gap-3">
                <Avatar size='md'>
                  <Avatar.Fallback>FK</Avatar.Fallback>
                </Avatar>
                <div className="flex flex-col">
                  <Typography.Paragraph size="sm" weight="medium">
                    Frantz Kati
                  </Typography.Paragraph>
                  <Typography.Paragraph size="xs" color="muted">
                    frantz@example.com
                  </Typography.Paragraph>
                </div>
              </Popover.Trigger>
              <Popover.Content className={'w-68 rounded-3xl border border-border'}>
                <Popover.Dialog className="flex flex-col gap-4 w-full p-0">
                  <div className="flex items-center gap-2 px-2 py-3 pb-0">
                    <Avatar size="md">
                      <Avatar.Fallback>FK</Avatar.Fallback>
                    </Avatar>
                    <div className="flex flex-col gap-0">
                      <Typography.Heading level={6}>
                        Frantz Kati
                      </Typography.Heading>
                      <Typography.Paragraph size="sm" color="muted">
                        frantz@example.com
                      </Typography.Paragraph>
                    </div>
                  </div>
                  <Separator className="my-0" />
                  <div className="flex flex-col gap-1 px-2 py-3 pt-0">
                    <Button variant="ghost" className="justify-start w-full">
                      Profile
                    </Button>
                    <Button variant="ghost" className="justify-start w-full">
                      Settings
                    </Button>
                    <Button variant="ghost" className="justify-start w-full">
                      Sign out
                    </Button>
                  </div>
                </Popover.Dialog>
              </Popover.Content>
            </Popover>
          </Card.Content>
        </Card>
      </div>

      <div className="h-screen w-full flex-1 p-2">
        <div className="h-full w-full">
          <div className="mx-auto flex h-full w-full max-w-4xl flex-col items-center pt-48">
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
    </div>
  )
}
