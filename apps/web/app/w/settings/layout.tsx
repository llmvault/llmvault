"use client"

import { useState } from "react"
import Link from "next/link"
import { usePathname } from "next/navigation"
import { AppIcon } from "@/components/icon"
import { cn } from "@/lib/utils"
import { AuthProvider } from "@/lib/auth/auth-context"
import { useIsAdmin } from "@/lib/auth/use-role"
import { NAV_SECTIONS, settingsHref } from "./_components/nav"

export default function SettingsLayout({
  children,
}: {
  children: React.ReactNode
}) {
  return (
    <AuthProvider>
      <SettingsChrome>{children}</SettingsChrome>
    </AuthProvider>
  )
}

function SettingsChrome({ children }: { children: React.ReactNode }) {
  const pathname = usePathname()
  const isAdmin = useIsAdmin()
  const [query, setQuery] = useState("")

  const sections = NAV_SECTIONS.map((section) => ({
    ...section,
    items: section.items.filter(
      (item) =>
        (!item.adminOnly || isAdmin) &&
        item.label.toLowerCase().includes(query.trim().toLowerCase())
    ),
  })).filter((section) => section.items.length > 0)

  return (
    <div className="fixed inset-0 flex overflow-clip bg-background text-foreground">
      <div className="flex w-72 shrink-0 flex-col border-r border-border bg-surface">
        <div className="flex flex-col gap-3 px-4 pb-2 pt-5">
          <Link
            href="/w"
            className="flex items-center gap-2 text-sm text-muted transition-colors hover:text-foreground"
          >
            <AppIcon icon="arrow-left" className="h-4 w-4" />
            Back to app
          </Link>
          <div className="flex items-center gap-2 rounded-xl border border-border bg-field-background px-3 py-1.5">
            <AppIcon icon="search" className="h-4 w-4 shrink-0 text-muted" />
            <input
              type="text"
              value={query}
              placeholder="Search settings..."
              aria-label="Search settings"
              onChange={(event) => setQuery(event.target.value)}
              className="min-w-0 flex-1 bg-transparent text-sm outline-none placeholder:text-muted"
            />
          </div>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto px-3 pb-4">
          {sections.map((section) => (
            <div key={section.label} className="flex flex-col gap-0.5 pt-4">
              <span className="px-3 pb-1 text-xs text-muted select-none">
                {section.label}
              </span>
              {section.items.map((item) => {
                const href = settingsHref(item.id)
                const active =
                  pathname === href || pathname.startsWith(`${href}/`)
                return (
                  <Link
                    key={item.id}
                    href={href}
                    className={cn(
                      "flex items-center gap-2.5 rounded-lg px-3 py-1.5 text-left text-sm transition-colors",
                      active ? "bg-default" : "hover:bg-default"
                    )}
                  >
                    <AppIcon icon={item.icon} className="h-4 w-4 shrink-0 text-muted" />
                    <span className="min-w-0 flex-1 truncate">{item.label}</span>
                  </Link>
                )
              })}
            </div>
          ))}
          {sections.length === 0 ? (
            <p className="px-3 pt-6 text-sm text-muted">No settings match</p>
          ) : null}
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-2xl px-6 py-12">{children}</div>
      </div>
    </div>
  )
}
