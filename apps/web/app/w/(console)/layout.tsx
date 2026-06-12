"use client"

import Link from "next/link"
import { usePathname, useRouter } from "next/navigation"
import { useEffect, useState, useCallback, useRef } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  LayoutDashboard,
  Chat01Icon,
  Plug01Icon,
  CommandIcon,
  Settings02Icon,
  DriveIcon,
  Chart01Icon,
  CreditCardIcon,
  CustomerService01Icon,
  AwardIcon,
  UserAdd01Icon,
} from "@hugeicons/core-free-icons"
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  useSidebar,
} from "@/components/ui/sidebar"
import { FullPageLoader } from "@/components/full-page-loader"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  DashboardThemeProvider,
  DashboardThemeSwitcher,
} from "@/components/dashboard-theme-switcher"
import { toast } from "sonner"
import { $api } from "@/lib/api/hooks"
import { AuthProvider, useAuth } from "@/lib/auth/auth-context"
import { cn } from "@/lib/utils"
import { UpgradeBanner } from "./_components/upgrade-banner"
import { NavUser } from "@/components/nav-user"

const navSections = [
  {
    label: "Workspace",
    items: [
      { label: "Dashboard", href: "/w", icon: LayoutDashboard },
      { label: "Sessions", href: "/w/sessions", icon: Chat01Icon },
    ],
  },
  {
    label: "Resources",
    items: [
      { label: "Drive", href: "/w/drive", icon: DriveIcon },
      { label: "Skills", href: "/w/skills", icon: CommandIcon },
    ],
  },
  {
    label: "Integrations",
    items: [{ label: "Connections", href: "/w/connections", icon: Plug01Icon }],
  },
  {
    label: "Admin",
    items: [
      { label: "Usage", href: "/w/usage", icon: Chart01Icon },
      { label: "Credits", href: "/w/credits", icon: CreditCardIcon },
      { label: "Settings", href: "/w/settings", icon: Settings02Icon },
    ],
  },
]

const footerLinks = [
  {
    label: "Support",
    href: "mailto:hello@usehivy.com",
    icon: CustomerService01Icon,
  },
  { label: "Get free credits", href: "#", icon: AwardIcon },
  { label: "Teams", href: "/w/teams", icon: UserAdd01Icon },
]

const collapsedSidebarRoutes = ["/w/sessions"]
const fullHeightWorkspaceRoutes = ["/w/sessions"]

function isWorkspaceRoute(pathname: string, routes: string[]) {
  return routes.some(
    (route) => pathname === route || pathname.startsWith(`${route}/`)
  )
}

function GhostLogo({
  className,
  size = 24,
}: {
  className?: string
  size?: number
}) {
  return (
    <svg
      viewBox="0 0 640 640"
      width={size}
      height={size}
      fill="currentColor"
      className={className}
    >
      <path d="M63.7314 260.875C115.623 104.119 238.334 51.5019 291.736 44.0986C600.403 1.30772 662.211 304.136 543.862 460.66C441.808 595.633 262.075 620.78 154.214 585.754C59.2103 554.903 6.44755 433.92 63.7314 260.875Z" />
      <ellipse
        cx="318.5"
        cy="282"
        rx="45.5"
        ry="101"
        fill="var(--background)"
      />
      <ellipse
        cx="457.5"
        cy="282"
        rx="45.5"
        ry="101"
        fill="var(--background)"
      />
    </svg>
  )
}

export default function WorkspaceV2Layout({
  children,
}: {
  children: React.ReactNode
}) {
  return (
    <AuthProvider>
      <WorkspaceGate>
        <DashboardThemeProvider>
          <SidebarProvider>
            <WorkspaceRouteSidebarState />
            <AppSidebar />
            <WorkspaceFrame>{children}</WorkspaceFrame>
          </SidebarProvider>
        </DashboardThemeProvider>
      </WorkspaceGate>
    </AuthProvider>
  )
}

function WorkspaceFrame({ children }: { children: React.ReactNode }) {
  const pathname = usePathname()
  const fullHeight = isWorkspaceRoute(pathname, fullHeightWorkspaceRoutes)

  return (
    <SidebarInset className="relative h-screen overflow-hidden">
      {/* Subtle top-right blur glow */}
      <div
        className="pointer-events-none absolute -top-32 -right-32 h-[500px] w-[500px] rounded-full opacity-30 blur-[120px]"
        style={{ backgroundColor: "var(--glow-right)" }}
      />
      <main
        className={cn(
          "relative z-10 flex h-full flex-1 flex-col",
          fullHeight ? "overflow-hidden" : "overflow-y-auto"
        )}
      >
        <UpgradeBanner />
        <div
          className={cn(
            "flex flex-1 flex-col",
            fullHeight ? "min-h-0 overflow-hidden" : "p-6 md:p-8"
          )}
        >
          {children}
        </div>
      </main>
    </SidebarInset>
  )
}

function WorkspaceRouteSidebarState() {
  const pathname = usePathname()
  const { open, setOpen, isMobile } = useSidebar()
  const restoreOpenRef = useRef<boolean | null>(null)
  const setOpenRef = useRef(setOpen)
  const shouldCollapse = isWorkspaceRoute(pathname, collapsedSidebarRoutes)

  useEffect(() => {
    setOpenRef.current = setOpen
  }, [setOpen])

  useEffect(() => {
    if (isMobile) return

    if (shouldCollapse) {
      if (restoreOpenRef.current === null) {
        restoreOpenRef.current = open
      }
      if (open) {
        setOpen(false)
      }
      return
    }

    if (restoreOpenRef.current !== null) {
      const restoreOpen = restoreOpenRef.current
      restoreOpenRef.current = null
      if (open !== restoreOpen) {
        setOpen(restoreOpen)
      }
    }
  }, [isMobile, open, setOpen, shouldCollapse])

  useEffect(() => {
    return () => {
      if (restoreOpenRef.current !== null) {
        setOpenRef.current(restoreOpenRef.current)
        restoreOpenRef.current = null
      }
    }
  }, [])

  return null
}

function WorkspaceGate({ children }: { children: React.ReactNode }) {
  const { user, activeOrg, isLoading } = useAuth()
  const router = useRouter()

  const needsEmailConfirmation = user !== null && !user.email_confirmed
  const needsOnboarding = activeOrg !== null && !activeOrg.onboarded

  useEffect(() => {
    if (!needsEmailConfirmation && needsOnboarding) {
      router.replace("/onboarding")
    }
  }, [needsEmailConfirmation, needsOnboarding, router])

  if (isLoading || !user) {
    return <FullPageLoader description="Loading workspace" />
  }

  if (needsOnboarding) {
    return <FullPageLoader description="Loading workspace" />
  }

  return (
    <>
      {needsEmailConfirmation && <EmailConfirmationGate />}
      {children}
    </>
  )
}

function EmailConfirmationGate() {
  const { user } = useAuth()
  const queryClient = useQueryClient()
  const [code, setCode] = useState("")
  const email = user?.email ?? ""

  const confirmMutation = $api.useMutation("post", "/auth/confirm-email")
  const resendMutation = $api.useMutation("post", "/auth/resend-confirmation")

  const handleConfirm = useCallback(() => {
    if (!code.trim()) return
    confirmMutation.mutate(
      { body: { email, code: code.trim() } },
      {
        onSuccess: () => {
          queryClient.invalidateQueries({ queryKey: ["get", "/auth/me"] })
        },
        onError: () => {
          toast.error("Invalid or expired code")
        },
      }
    )
  }, [code, email, confirmMutation, queryClient])

  const handleResend = useCallback(() => {
    resendMutation.mutate(
      { body: { email } },
      {
        onSuccess: () => toast.success("A new confirmation code has been sent"),
        onError: () => toast.error("Could not resend confirmation"),
      }
    )
  }, [email, resendMutation])

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "Enter") handleConfirm()
    },
    [handleConfirm]
  )

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm">
      <div className="w-full max-w-md rounded-xl border border-border bg-background p-8 shadow-2xl">
        <h2 className="font-heading text-xl font-semibold text-foreground">
          Confirm your email
        </h2>
        <p className="mt-2 text-sm text-muted-foreground">
          We sent a 6-digit code to{" "}
          <span className="font-medium text-foreground">{email}</span>. Enter it
          below to continue.
        </p>

        <div className="mt-6 space-y-4">
          <Input
            type="text"
            inputMode="numeric"
            maxLength={6}
            placeholder="000000"
            value={code}
            onChange={(e) =>
              setCode(e.target.value.replace(/\D/g, "").slice(0, 6))
            }
            onKeyDown={handleKeyDown}
            className="text-center font-mono text-2xl tracking-[0.3em]"
            autoFocus
          />

          <Button
            onClick={handleConfirm}
            disabled={code.length !== 6}
            loading={confirmMutation.isPending}
            className="w-full"
          >
            Verify
          </Button>
        </div>

        <div className="mt-4 text-center">
          <button
            onClick={handleResend}
            disabled={resendMutation.isPending}
            className="text-sm text-muted-foreground underline underline-offset-4 transition-colors hover:text-foreground disabled:opacity-50"
          >
            {resendMutation.isPending ? "Sending..." : "Resend code"}
          </button>
        </div>
      </div>
    </div>
  )
}

function AppSidebar() {
  const pathname = usePathname()

  return (
    <Sidebar
      collapsible="icon"
      className="h-svh min-h-svh border-r border-border"
    >
      <SidebarHeader>
        <div className="flex items-center gap-2.5 px-4 py-4">
          <GhostLogo className="text-primary" />
          <span className="font-heading text-lg font-medium tracking-tight text-sidebar-foreground group-data-[collapsible=icon]:hidden">
            hivy
          </span>
        </div>
      </SidebarHeader>

      <SidebarContent className="gap-5 px-3 py-2">
        {navSections.map((section) => (
          <div key={section.label}>
            <p className="px-3 pb-1.5 text-[11px] font-semibold tracking-wide text-sidebar-foreground/40 uppercase group-data-[collapsible=icon]:hidden">
              {section.label}
            </p>
            <SidebarMenu>
              {section.items.map((item) => {
                const active =
                  pathname === item.href ||
                  (item.href !== "/w" && pathname.startsWith(`${item.href}/`))

                return (
                  <SidebarMenuItem key={item.href}>
                    <SidebarMenuButton
                      render={<Link href={item.href} />}
                      isActive={active}
                      tooltip={item.label}
                      className={cn(
                        "h-11 cursor-pointer items-center gap-3 rounded-md font-display text-base",
                        active && "text-primary"
                      )}
                    >
                      <HugeiconsIcon icon={item.icon} size={16} />
                      <span>{item.label}</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                )
              })}
            </SidebarMenu>
          </div>
        ))}
      </SidebarContent>

      <SidebarFooter className="gap-0 px-3 py-4">
        <div className="mb-3">
          <DashboardThemeSwitcher />
        </div>
        <SidebarMenu>
          {footerLinks.map((link) => (
            <SidebarMenuItem key={link.label}>
              <SidebarMenuButton
                render={<Link href={link.href} />}
                tooltip={link.label}
                className={cn(
                  "h-11 cursor-pointer items-center gap-3 rounded-md font-display text-base"
                )}
              >
                <HugeiconsIcon icon={link.icon} size={16} />
                <span>{link.label}</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          ))}
        </SidebarMenu>

        <div className="mt-4">
          <NavUser />
        </div>
      </SidebarFooter>
    </Sidebar>
  )
}
