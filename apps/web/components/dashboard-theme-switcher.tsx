"use client"

import { useEffect, useMemo, useState, type ReactNode } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { PaintBoardIcon } from "@hugeicons/core-free-icons"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { cn } from "@/lib/utils"

const STORAGE_KEY = "hivy-dashboard-theme"

const dashboardThemes = [
  { id: "rose", name: "Rose", from: "#FB7185", to: "#FECDD3" },
  { id: "lavender", name: "Lavender", from: "#8B8CF6", to: "#F472B6" },
  { id: "ocean", name: "Ocean", from: "#38BDF8", to: "#818CF8" },
  { id: "slate", name: "Slate", from: "#94A3B8", to: "#E2E8F0" },
  { id: "coral", name: "Coral", from: "#FB923C", to: "#FECACA" },
  { id: "forest", name: "Forest", from: "#86A586", to: "#C1D9C1" },
  { id: "midnight", name: "Midnight", from: "#60A5FA", to: "#A78BFA" },
  { id: "sand", name: "Sand", from: "#C4A484", to: "#E6D5B8" },
  { id: "sky", name: "Sky", from: "#38BDF8", to: "#BAE6FD" },
  { id: "amber", name: "Amber", from: "#F59E0B", to: "#FDE68A" },
  { id: "berry", name: "Berry", from: "#A855F7", to: "#E9D5FF" },
  { id: "mint", name: "Mint", from: "#34D399", to: "#A7F3D0" },
] as const

type DashboardTheme = (typeof dashboardThemes)[number]["id"]

function isDashboardTheme(value: string | null): value is DashboardTheme {
  return dashboardThemes.some((theme) => theme.id === value)
}

export function DashboardThemeProvider({
  children,
}: {
  children: ReactNode
}) {
  useEffect(() => {
    const stored = window.localStorage.getItem(STORAGE_KEY)
    const theme = isDashboardTheme(stored) ? stored : "rose"
    document.documentElement.dataset.dashboardTheme = theme

    return () => {
      delete document.documentElement.dataset.dashboardTheme
    }
  }, [])

  return <>{children}</>
}

export function DashboardThemeSwitcher() {
  const [theme, setTheme] = useState<DashboardTheme>("rose")

  useEffect(() => {
    const stored = window.localStorage.getItem(STORAGE_KEY)
    const nextTheme = isDashboardTheme(stored) ? stored : "rose"
    setTheme(nextTheme)
    document.documentElement.dataset.dashboardTheme = nextTheme
  }, [])

  const activeTheme = useMemo(
    () => dashboardThemes.find((item) => item.id === theme) ?? dashboardThemes[0],
    [theme]
  )

  function handleThemeChange(value: string) {
    if (!isDashboardTheme(value)) return
    setTheme(value)
    window.localStorage.setItem(STORAGE_KEY, value)
    document.documentElement.dataset.dashboardTheme = value
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant="outline"
            size="sm"
            className="h-10 w-full justify-start gap-2 border-sidebar-border bg-sidebar-accent/40 px-3 text-sidebar-foreground hover:bg-sidebar-accent group-data-[collapsible=icon]:size-10 group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0"
            aria-label="Dashboard theme"
          />
        }
      >
        <HugeiconsIcon icon={PaintBoardIcon} strokeWidth={2} />
        <span
          className="size-3 rounded-full border border-sidebar-border group-data-[collapsible=icon]:hidden"
          style={{
            background: `linear-gradient(135deg, ${activeTheme.from}, ${activeTheme.to})`,
          }}
        />
        <span className="truncate group-data-[collapsible=icon]:hidden">
          {activeTheme.name}
        </span>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        side="right"
        align="end"
        sideOffset={8}
        className="w-56"
      >
        <DropdownMenuGroup>
          <DropdownMenuLabel>Dashboard theme</DropdownMenuLabel>
          <DropdownMenuRadioGroup value={theme} onValueChange={handleThemeChange}>
            {dashboardThemes.map((item) => (
              <DropdownMenuRadioItem key={item.id} value={item.id}>
                <span
                  className={cn(
                    "size-4 shrink-0 rounded-full border",
                    item.id === theme ? "border-foreground" : "border-border"
                  )}
                  style={{
                    background: `linear-gradient(135deg, ${item.from}, ${item.to})`,
                  }}
                />
                {item.name}
              </DropdownMenuRadioItem>
            ))}
          </DropdownMenuRadioGroup>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
