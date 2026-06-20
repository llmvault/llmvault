"use client"

import { useState } from "react"
import { useRouter } from "next/navigation"
import { Popover } from "@heroui/react"
import { Icon } from "@iconify/react"
import { useAuth } from "@/lib/auth/auth-context"

export function AccountMenu() {
  const [open, setOpen] = useState(false)
  const [loggingOut, setLoggingOut] = useState(false)
  const router = useRouter()
  const { user, activeOrg, logout } = useAuth()

  const go = (path?: string) => {
    setOpen(false)
    if (path) router.push(path)
  }

  const handleLogout = async () => {
    setOpen(false)
    setLoggingOut(true)
    await logout()
  }

  return (
    <Popover isOpen={open} onOpenChange={setOpen}>
      <Popover.Trigger
        aria-label="Account and settings"
        className="hover:bg-default flex flex-1 items-center gap-2.5 rounded-lg px-3 py-1.5 text-left text-sm transition-colors"
      >
        <Icon icon="lucide:settings" className="h-4 w-4 shrink-0 text-muted" />
        <span className="min-w-0 flex-1 truncate">Settings</span>
      </Popover.Trigger>
      <Popover.Content className="w-64 rounded-2xl border border-border p-1.5">
        <Popover.Dialog className="flex w-full flex-col gap-0.5 p-0">
          <div className="flex flex-col gap-1 px-2.5 pt-1.5 pb-2">
            <div className="flex items-center gap-2 text-sm text-muted">
              <Icon icon="lucide:circle-user" className="h-4 w-4 shrink-0" />
              <span className="truncate">{user?.email ?? "Signed in"}</span>
            </div>
            <div className="flex items-center gap-2 text-sm text-muted">
              <Icon icon="lucide:settings" className="h-4 w-4 shrink-0" />
              <span className="truncate">{activeOrg?.name ?? "Workspace"}</span>
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
            label={loggingOut ? "Logging out..." : "Log out"}
            disabled={loggingOut}
            onPress={handleLogout}
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
  disabled,
  onPress,
}: {
  icon: string
  label: string
  shortcut?: string
  chevron?: boolean
  disabled?: boolean
  onPress: () => void | Promise<void>
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onPress}
      className="hover:bg-default flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors disabled:cursor-progress disabled:opacity-60"
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
