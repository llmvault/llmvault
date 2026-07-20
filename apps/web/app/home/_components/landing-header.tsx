"use client"

import { NavigationMenu } from "@base-ui/react/navigation-menu"
import { Button, Link } from "@heroui/react"
import { AppIcon } from "@/components/icon"

const platformLinks = [
  {
    title: "Knowledge base",
    description: "Ground agents in approved organizational knowledge.",
    href: "/w/settings/knowledge",
    icon: "brain",
  },
  {
    title: "Agents",
    description: "Build, configure, and manage your AI workforce.",
    href: "/w/agents",
    icon: "bot",
  },
  {
    title: "Drive",
    description: "Store and organize the files agents create and use.",
    href: "/w",
    icon: "folder",
  },
  {
    title: "Automations",
    description: "Run agents on schedules, webhooks, and connection triggers.",
    href: "/w/automations",
    icon: "workflow",
  },
  {
    title: "Sheets",
    description: "Structured databases agents can read and update.",
    href: "/w/sheets",
    icon: "table",
  },
  {
    title: "Access control",
    description: "Manage roles, teams, and resource permissions.",
    href: "/w/settings/teams",
    icon: "shield-check",
  },
] as const

const resourceLinks = [
  {
    title: "Docs",
    description: "Learn how to build and operate Hivy.",
    href: "/docs",
    icon: "file-text",
  },
  {
    title: "Blog",
    description: "Product ideas, guides, and field notes.",
    href: "/blog",
    icon: "scroll",
  },
  {
    title: "Changelog",
    description: "Follow improvements across Hivy.",
    href: "/changelog",
    icon: "activity",
  },
  {
    title: "Models",
    description: "Compare the models available to your agents.",
    href: "/models",
    icon: "brain",
  },
] as const

type NavigationLink =
  | (typeof platformLinks)[number]
  | (typeof resourceLinks)[number]

const triggerClassName =
  "group flex h-8 items-center gap-1.5 rounded-sm px-2 text-[0.82rem] text-foreground/80 outline-none transition-colors duration-150 hover:bg-surface-secondary hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring data-[popup-open]:bg-surface-secondary data-[popup-open]:text-foreground"

function MenuLink({ item }: { item: NavigationLink }) {
  return (
    <NavigationMenu.Link
      closeOnClick
      render={<Link href={item.href} />}
      className="marketing-menu-link group focus-visible:ring-ring flex min-h-20 items-start gap-3 rounded-sm px-3 py-3 text-foreground transition-colors duration-150 outline-none hover:bg-surface-secondary focus-visible:bg-surface-secondary focus-visible:ring-2"
    >
      <span className="flex size-9 shrink-0 items-center justify-center rounded-sm border border-border bg-surface-secondary text-muted transition-colors duration-150 group-hover:text-foreground">
        <AppIcon icon={item.icon} size={17} />
      </span>
      <span className="min-w-0 pt-0.5">
        <span className="block text-sm leading-5 font-medium tracking-[-0.01em]">
          {item.title}
        </span>
        <span className="mt-1 block text-xs leading-[1.45] text-muted lg:whitespace-nowrap">
          {item.description}
        </span>
      </span>
    </NavigationMenu.Link>
  )
}

function MenuTrigger({ children }: { children: string }) {
  return (
    <NavigationMenu.Trigger className={triggerClassName}>
      {children}
      <NavigationMenu.Icon className="transition-transform duration-150 ease-out data-[popup-open]:rotate-180">
        <AppIcon icon="chevron-down" size={12} />
      </NavigationMenu.Icon>
    </NavigationMenu.Trigger>
  )
}

export function LandingHeader() {
  return (
    <header className="mx-auto grid h-16 w-[calc(100%-2rem)] max-w-[1300px] grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] items-center">
      <Link
        href="/home"
        aria-label="Hivy home"
        className="w-fit text-[1.05rem] font-semibold tracking-[-0.05em] text-foreground"
      >
        hivy
      </Link>

      <NavigationMenu.Root
        aria-label="Main navigation"
        className="hidden md:block"
        delay={70}
        closeDelay={100}
      >
        <NavigationMenu.List className="flex items-center gap-1">
          <NavigationMenu.Item value="platform">
            <MenuTrigger>Platform</MenuTrigger>
            <NavigationMenu.Content
              keepMounted
              className="w-[calc(100vw-2rem)] p-2 lg:w-max lg:max-w-[calc(100vw-2rem)]"
            >
              <div className="px-3 pt-2 pb-1 text-[0.68rem] font-medium tracking-[0.08em] text-muted uppercase">
                Platform
              </div>
              <ul className="grid grid-cols-2 lg:grid-cols-[max-content_max-content]">
                {platformLinks.map((item) => (
                  <li key={item.title}>
                    <MenuLink item={item} />
                  </li>
                ))}
              </ul>
            </NavigationMenu.Content>
          </NavigationMenu.Item>

          <NavigationMenu.Item value="resources">
            <MenuTrigger>Resources</MenuTrigger>
            <NavigationMenu.Content
              keepMounted
              className="w-[calc(100vw-2rem)] p-2 lg:w-max lg:max-w-[calc(100vw-2rem)]"
            >
              <div className="px-3 pt-2 pb-1 text-[0.68rem] font-medium tracking-[0.08em] text-muted uppercase">
                Resources
              </div>
              <ul className="grid grid-cols-2 lg:grid-cols-[max-content_max-content]">
                {resourceLinks.map((item) => (
                  <li key={item.title}>
                    <MenuLink item={item} />
                  </li>
                ))}
              </ul>
            </NavigationMenu.Content>
          </NavigationMenu.Item>

          <NavigationMenu.Item>
            <NavigationMenu.Link
              render={<Link href="/pricing" />}
              className={`${triggerClassName} inline-flex`}
            >
              Pricing
            </NavigationMenu.Link>
          </NavigationMenu.Item>

          <NavigationMenu.Item>
            <NavigationMenu.Link
              render={<Link href="https://github.com/usehivy/hivy" />}
              className={`${triggerClassName} inline-flex`}
            >
              <AppIcon
                icon="github"
                size={14}
                className="size-3.5 shrink-0 [&_path]:fill-current"
              />
              0.1k
            </NavigationMenu.Link>
          </NavigationMenu.Item>
        </NavigationMenu.List>

        <NavigationMenu.Portal className="marketing-link-scope light">
          <NavigationMenu.Positioner
            sideOffset={8}
            align="center"
            collisionPadding={16}
            className="z-50 outline-none"
          >
            <NavigationMenu.Popup className="h-[var(--popup-height)] w-[var(--popup-width)] origin-[var(--transform-origin)] overflow-hidden rounded-sm border border-border bg-background text-foreground shadow-lg transition-[width,height,opacity,transform] duration-200 ease-[cubic-bezier(0.16,1,0.3,1)] outline-none data-[ending-style]:-translate-y-1 data-[ending-style]:opacity-0 data-[starting-style]:-translate-y-1 data-[starting-style]:opacity-0">
              <NavigationMenu.Viewport />
            </NavigationMenu.Popup>
          </NavigationMenu.Positioner>
        </NavigationMenu.Portal>
      </NavigationMenu.Root>

      <div className="flex items-center justify-end gap-2">
        <Link href="/auth/login" className="hidden sm:inline-flex">
          <Button size="sm" variant="ghost">
            Log in
          </Button>
        </Link>
        <Link href="/auth/signup">
          <Button size="sm">Start for free</Button>
        </Link>
      </div>
    </header>
  )
}
