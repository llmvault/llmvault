"use client"

import { usePathname, useRouter } from "next/navigation"
import { useEffect } from "react"
import {
  Avatar,
  Button,
  Card,
  Chip,
  ListBox,
  Popover,
  Select,
  Separator,
  Typography,
} from "@heroui/react"
import { HugeiconsIcon } from "@hugeicons/react"
import { PlusSignIcon } from "@hugeicons/core-free-icons"
import { Icon } from "@iconify/react"
import { ClaudeIcon } from "./_components/claude"
import { DeepseekIcon } from "./_components/deepseek"
import { GeminiIcon } from "./_components/gemini"
import { GrokIcon } from "./_components/grok"
import { MoonshotIcon } from "./_components/moonshot"
import { QwenIcon } from "./_components/qwen"
import { XaiIcon } from "./_components/xai"
import { FullPageLoader } from "@/components/full-page-loader"
import { AuthProvider, useAuth } from "@/lib/auth/auth-context"

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
  { id: "general", name: "general", private: false, unread: false, unreadCount: 0 },
  { id: "random", name: "random", private: false, unread: true, unreadCount: 0 },
  { id: "engineering", name: "engineering", private: false, unread: true, unreadCount: 3 },
  { id: "design", name: "design", private: false, unread: false, unreadCount: 0 },
  { id: "exec-updates", name: "exec-updates", private: true, unread: true, unreadCount: 1 },
  { id: "deploys", name: "deploys", private: false, unread: false, unreadCount: 0 },
]

export default function WLayout({
  children,
}: {
  children: React.ReactNode
}) {
  return (
    <AuthProvider signInPath="/hero/auth/login">
      <HeroWorkspaceGate>
        <HeroWorkspaceShell>{children}</HeroWorkspaceShell>
      </HeroWorkspaceGate>
    </AuthProvider>
  )
}

function HeroWorkspaceGate({ children }: { children: React.ReactNode }) {
  const { user, activeOrg, isLoading } = useAuth()
  const router = useRouter()
  const needsOnboarding = activeOrg !== null && !activeOrg.onboarded

  useEffect(() => {
    if (needsOnboarding) {
      router.replace("/onboarding")
    }
  }, [needsOnboarding, router])

  if (isLoading || !user || needsOnboarding) {
    return <FullPageLoader description="Loading workspace" />
  }

  return children
}

function initials(name?: string | null) {
  if (!name) return "?"
  const parts = name.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) return "?"
  if (parts.length === 1) return parts[0].charAt(0).toUpperCase()
  return (parts[0].charAt(0) + parts[parts.length - 1].charAt(0)).toUpperCase()
}

function HeroWorkspaceShell({
  children,
}: {
  children: React.ReactNode
}) {
  const pathname = usePathname()
  const router = useRouter()
  const { user, logout } = useAuth()
  const activeChannelId = pathname?.split("/channels/")[1] || ""
  const name = user?.name ?? user?.email ?? "Account"
  const email = user?.email ?? ""
  const fallback = initials(name)

  return (
    <div className="flex h-screen w-screen">
      <div className="h-screen w-80 p-2 pr-0">
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
                  {channels.map((channel) => {
                    const isActive = channel.id === activeChannelId
                    return (
                      <Button
                        key={channel.id}
                        variant="ghost"
                        className={`h-auto w-full justify-start gap-2 px-3 py-1.5 ${isActive ? "bg-accent" : ""}`}
                        onPress={() => router.push(`/hero/w/channels/${channel.id}`)}
                      >
                        <Icon
                          icon={channel.private ? "lucide:lock" : "lucide:hash"}
                          className={`h-4 w-4 ${isActive ? "text-accent-foreground" : ""}`}
                        />
                        <Typography.Paragraph
                          size="sm"
                          color="muted"
                          className={`flex-1 ${isActive ? "text-accent-foreground" : ""}`}
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
                  })}
                </div>
              </div>
            </div>

            {/* User avatar — stuck to bottom */}
            <Popover>
              <Popover.Trigger className="flex cursor-pointer items-center gap-3">
                <Avatar size="md">
                  <Avatar.Fallback>{fallback}</Avatar.Fallback>
                </Avatar>
                <div className="flex flex-col">
                  <Typography.Paragraph size="sm" weight="medium">
                    {name}
                  </Typography.Paragraph>
                  {email ? (
                    <Typography.Paragraph size="xs" color="muted">
                      {email}
                    </Typography.Paragraph>
                  ) : null}
                </div>
              </Popover.Trigger>
              <Popover.Content className={"w-68 rounded-3xl border border-border"}>
                <Popover.Dialog className="flex w-full flex-col gap-4 p-0">
                  <div className="flex items-center gap-2 px-2 py-3 pb-0">
                    <Avatar size="md">
                      <Avatar.Fallback>{fallback}</Avatar.Fallback>
                    </Avatar>
                    <div className="flex flex-col gap-0">
                      <Typography.Heading level={6}>
                        {name}
                      </Typography.Heading>
                      {email ? (
                        <Typography.Paragraph size="sm" color="muted">
                          {email}
                        </Typography.Paragraph>
                      ) : null}
                    </div>
                  </div>
                  <Separator className="my-0" />
                  <div className="flex flex-col gap-1 px-2 py-3 pt-0">
                    <Button
                      variant="ghost"
                      className="w-full justify-start"
                      onPress={() => router.push("/w/settings/general")}
                    >
                      Profile
                    </Button>
                    <Button
                      variant="ghost"
                      className="w-full justify-start"
                      onPress={() => router.push("/w/settings/general")}
                    >
                      Settings
                    </Button>
                    <Button
                      variant="ghost"
                      className="w-full justify-start"
                      onPress={() => logout()}
                    >
                      Sign out
                    </Button>
                  </div>
                </Popover.Dialog>
              </Popover.Content>
            </Popover>
          </Card.Content>
        </Card>
      </div>


        {children}
    </div>
  )
}
