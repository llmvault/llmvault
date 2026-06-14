"use client"

import { useState } from "react"
import { useRouter } from "next/navigation"
import { AnimatePresence, motion } from "motion/react"
import { Button, Popover, Typography } from "@heroui/react"
import { Icon } from "@iconify/react"
import { projects, type SidebarProject } from "../_lib/static-data"
import { DEFAULT_AGENT_ID } from "../_lib/agents"
import { useWorkspace } from "./shell"

export function Sidebar({ onCollapse }: { onCollapse: () => void }) {
  const { startNewChat } = useWorkspace()
  const router = useRouter()

  return (
    <div className="flex h-full flex-col bg-surface">
      <div className="flex h-12 shrink-0 items-center gap-1 px-3">
        <Button
          variant="ghost"
          size="sm"
          isIconOnly
          aria-label="Collapse sidebar"
          onPress={onCollapse}
        >
          <Icon icon="lucide:panel-left" className="h-4 w-4 text-muted" />
        </Button>
        <Button variant="ghost" size="sm" isIconOnly aria-label="Back">
          <Icon icon="lucide:arrow-left" className="h-4 w-4 text-muted" />
        </Button>
        <Button variant="ghost" size="sm" isIconOnly aria-label="Forward">
          <Icon icon="lucide:arrow-right" className="h-4 w-4 text-muted/50" />
        </Button>
      </div>

      <div className="flex min-h-0 flex-1 flex-col gap-6 overflow-y-auto px-3 pb-4">
        <div className="flex flex-col gap-0.5">
          <NavRow icon="lucide:square-pen" label="New chat" onClick={startNewChat} />
          <NavRow icon="lucide:search" label="Search" />
          <NavRow
            icon="lucide:toy-brick"
            label="Plugins"
            onClick={() => router.push("/w/plugins")}
          />
          <NavRow icon="lucide:clock" label="Automations" />
        </div>

        <div className="flex flex-col gap-0.5">
          <SectionLabel>Projects</SectionLabel>
          {projects.map((project) => (
            <ProjectGroup key={project.name} project={project} />
          ))}
        </div>

        <div className="flex flex-col gap-0.5">
          <SectionLabel>Chats</SectionLabel>
        </div>
      </div>

      <div className="shrink-0 border-t border-border px-3 py-2">
        <div className="flex items-center">
          <AccountMenu />
          <Button variant="ghost" size="sm" isIconOnly aria-label="Mobile app">
            <Icon icon="lucide:smartphone" className="h-4 w-4 text-muted" />
          </Button>
        </div>
      </div>
    </div>
  )
}

function AccountMenu() {
  const [open, setOpen] = useState(false)
  const router = useRouter()

  const go = (path?: string) => {
    setOpen(false)
    if (path) router.push(path)
  }

  return (
    <Popover isOpen={open} onOpenChange={setOpen}>
      <Popover.Trigger
        aria-label="Account and settings"
        className="flex flex-1 items-center gap-2.5 rounded-lg px-3 py-1.5 text-left text-sm transition-colors hover:bg-default"
      >
        <Icon icon="lucide:settings" className="h-4 w-4 shrink-0 text-muted" />
        <span className="min-w-0 flex-1 truncate">Settings</span>
      </Popover.Trigger>
      <Popover.Content className="w-64 rounded-2xl border border-border p-1.5">
        <Popover.Dialog className="flex w-full flex-col gap-0.5 p-0">
          <div className="flex flex-col gap-1 px-2.5 pb-2 pt-1.5">
            <div className="flex items-center gap-2 text-sm text-muted">
              <Icon icon="lucide:circle-user" className="h-4 w-4 shrink-0" />
              <span className="truncate">bahdcoder@gmail.com</span>
            </div>
            <div className="flex items-center gap-2 text-sm text-muted">
              <Icon icon="lucide:settings" className="h-4 w-4 shrink-0" />
              <span className="truncate">Personal account</span>
            </div>
          </div>
          <div className="mx-1 border-t border-border" />
          <AccountItem
            icon="lucide:circle-user"
            label="Profile"
            onPress={() => go("/w/settings")}
          />
          <AccountItem
            icon="lucide:settings"
            label="Settings"
            shortcut="⌘,"
            onPress={() => go("/w/settings")}
          />
          <div className="mx-1 border-t border-border" />
          <AccountItem
            icon="lucide:gauge"
            label="Usage remaining"
            chevron
            onPress={() => go()}
          />
          <AccountItem
            icon="lucide:mail"
            label="Invite a friend"
            onPress={() => go()}
          />
          <AccountItem
            icon="lucide:log-out"
            label="Log out"
            onPress={() => go()}
          />
        </Popover.Dialog>
      </Popover.Content>
    </Popover>
  )
}

function AccountItem({
  icon,
  label,
  shortcut,
  chevron,
  onPress,
}: {
  icon: string
  label: string
  shortcut?: string
  chevron?: boolean
  onPress: () => void
}) {
  return (
    <button
      type="button"
      onClick={onPress}
      className="flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors hover:bg-default"
    >
      <Icon icon={icon} className="h-4 w-4 shrink-0 text-muted" />
      <span className="min-w-0 flex-1 truncate">{label}</span>
      {shortcut ? <span className="text-xs text-muted">{shortcut}</span> : null}
      {chevron ? (
        <Icon icon="lucide:chevron-right" className="h-3.5 w-3.5 text-muted" />
      ) : null}
    </button>
  )
}

const COLLAPSE_TRANSITION = {
  duration: 0.25,
  ease: [0.32, 0.72, 0, 1] as const,
}

function ProjectGroup({ project }: { project: SidebarProject }) {
  const [expanded, setExpanded] = useState(true)
  const [showingMore, setShowingMore] = useState(false)
  const { session, openChat } = useWorkspace()

  return (
    <div className="flex flex-col">
      <button
        type="button"
        onClick={() => setExpanded((open) => !open)}
        className="group flex items-center gap-2.5 rounded-lg px-3 py-1.5 text-left text-sm transition-colors hover:bg-default"
      >
        <span className="relative h-4 w-4 shrink-0">
          <Icon
            icon="lucide:folder"
            className="absolute inset-0 h-4 w-4 text-muted transition-opacity group-hover:opacity-0"
          />
          <Icon
            icon={expanded ? "lucide:chevron-down" : "lucide:chevron-right"}
            className="absolute inset-0 h-4 w-4 text-muted opacity-0 transition-opacity group-hover:opacity-100"
          />
        </span>
        <span className="min-w-0 flex-1 truncate">{project.name}</span>
      </button>

      <AnimatePresence initial={false}>
        {expanded ? (
          <motion.div
            key="chats"
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: "auto", opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={COLLAPSE_TRANSITION}
            className="overflow-hidden"
          >
            <div className="flex flex-col gap-0.5 pt-0.5">
              {project.chats.map((chat) => (
                <ChatRow
                  key={chat.title}
                  title={chat.title}
                  meta={chat.meta}
                  active={chat.title === session?.title}
                  onSelect={() =>
                    openChat(chat.title, chat.agentId ?? DEFAULT_AGENT_ID)
                  }
                />
              ))}
              <AnimatePresence initial={false}>
                {showingMore && project.moreChats?.length ? (
                  <motion.div
                    key="more"
                    initial={{ height: 0, opacity: 0 }}
                    animate={{ height: "auto", opacity: 1 }}
                    exit={{ height: 0, opacity: 0 }}
                    transition={COLLAPSE_TRANSITION}
                    className="overflow-hidden"
                  >
                    <div className="flex flex-col gap-0.5">
                      {project.moreChats.map((chat) => (
                        <ChatRow
                          key={chat.title}
                          title={chat.title}
                          meta={chat.meta}
                          active={chat.title === session?.title}
                          onSelect={() =>
                            openChat(chat.title, chat.agentId ?? DEFAULT_AGENT_ID)
                          }
                        />
                      ))}
                    </div>
                  </motion.div>
                ) : null}
              </AnimatePresence>
              {project.moreChats?.length ? (
                <button
                  type="button"
                  onClick={() => setShowingMore((showing) => !showing)}
                  className="rounded-lg px-3 py-1 pl-9 text-left text-sm text-muted transition-colors hover:bg-default"
                >
                  {showingMore ? "Show less" : "Show more"}
                </button>
              ) : null}
              {project.noChats ? (
                <span className="px-3 py-1 pl-9 text-sm text-muted/60">
                  No chats
                </span>
              ) : null}
            </div>
          </motion.div>
        ) : null}
      </AnimatePresence>
    </div>
  )
}

function SectionLabel({ children }: { children: React.ReactNode }) {
  return (
    <Typography.Paragraph
      size="xs"
      color="muted"
      className="px-3 pb-1 pt-2 select-none"
    >
      {children}
    </Typography.Paragraph>
  )
}

function NavRow({
  icon,
  label,
  className = "",
  onClick,
}: {
  icon: string
  label: string
  className?: string
  onClick?: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`flex items-center gap-2.5 rounded-lg px-3 py-1.5 text-left text-sm transition-colors hover:bg-default ${className}`}
    >
      <Icon icon={icon} className="h-4 w-4 shrink-0 text-muted" />
      <span className="min-w-0 flex-1 truncate">{label}</span>
    </button>
  )
}

function ChatRow({
  title,
  meta,
  active,
  onSelect,
}: {
  title: string
  meta: string
  active?: boolean
  onSelect: () => void
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      className={`flex items-center gap-2 rounded-lg py-1.5 pl-9 pr-3 text-left text-sm transition-colors ${
        active ? "bg-default" : "hover:bg-default"
      }`}
    >
      <span className="min-w-0 flex-1 truncate">{title}</span>
      <span className="shrink-0 text-xs text-muted">{meta}</span>
    </button>
  )
}
