"use client"

import { useState } from "react"
import { useRouter } from "next/navigation"
import { Popover } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { useAuth } from "@/lib/auth/auth-context"

export function AccountMenu() {
  const [open, setOpen] = useState(false)
  const [loggingOut, setLoggingOut] = useState(false)
  const router = useRouter()
  const { user, orgs, activeOrg, setActiveOrg, logout } = useAuth()

  const go = (path: string) => {
    setOpen(false)
    router.push(path)
  }

  const switchOrg = (org: (typeof orgs)[number]) => {
    setOpen(false)
    if (org.id !== activeOrg?.id) setActiveOrg(org)
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
        className="flex flex-1 items-center gap-2.5 rounded-lg px-3 py-1.5 text-left text-sm transition-colors hover:bg-default"
      >
        <AppIcon icon="settings" className="h-4 w-4 shrink-0 text-muted" />
        <span className="min-w-0 flex-1 truncate">Settings</span>
      </Popover.Trigger>
      <Popover.Content className="w-64 border border-border p-1.5">
        <Popover.Dialog className="flex w-full flex-col gap-0.5 p-0">
          <div className="flex flex-col gap-1 px-2.5 pt-1.5 pb-2">
            <div className="flex items-center gap-2 text-sm text-muted">
              <AppIcon icon="circle-user" className="h-4 w-4 shrink-0" />
              <span className="truncate">{user?.email ?? "Signed in"}</span>
            </div>
            <div className="flex items-center gap-2 text-sm text-muted">
              <AppIcon icon="settings" className="h-4 w-4 shrink-0" />
              <span className="truncate">{activeOrg?.name ?? "Workspace"}</span>
            </div>
          </div>
          {orgs.length > 1 ? (
            <>
              <div className="mx-1 border-t border-border" />
              <p className="px-2.5 pt-1.5 pb-1 text-[11px] font-medium tracking-wide text-muted uppercase">
                Switch workspace
              </p>
              {orgs.map((org) => (
                <button
                  key={org.id}
                  type="button"
                  onClick={() => switchOrg(org)}
                  className="flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors hover:bg-default"
                >
                  <AppIcon
                    icon={org.id === activeOrg?.id ? "check" : "settings"}
                    className="h-4 w-4 shrink-0 text-muted"
                  />
                  <span className="min-w-0 flex-1 truncate">
                    {org.name ?? "Workspace"}
                  </span>
                </button>
              ))}
            </>
          ) : null}
          <div className="mx-1 border-t border-border" />
          <AccountItem
            icon="circle-user"
            label="Profile"
            onPress={() => go("/w/settings")}
          />
          <AccountItem
            icon="settings"
            label="Settings"
            onPress={() => go("/w/settings")}
          />
          <div className="mx-1 border-t border-border" />
          <AccountItem
            icon="log-out"
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
  disabled,
  onPress,
}: {
  icon: string
  label: string
  disabled?: boolean
  onPress: () => void | Promise<void>
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onPress}
      className="flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors hover:bg-default disabled:cursor-progress disabled:opacity-60"
    >
      <AppIcon icon={icon} className="h-4 w-4 shrink-0 text-muted" />
      <span className="min-w-0 flex-1 truncate">{label}</span>
    </button>
  )
}
