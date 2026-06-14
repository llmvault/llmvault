export interface SettingsNavItem {
  id: string
  label: string
  icon: string
}

export interface SettingsNavSection {
  label: string
  items: SettingsNavItem[]
}

export const NAV_SECTIONS: SettingsNavSection[] = [
  {
    label: "Personal",
    items: [
      { id: "general", label: "General", icon: "lucide:settings" },
      { id: "profile", label: "Profile", icon: "lucide:circle-user" },
      { id: "appearance", label: "Appearance", icon: "lucide:sun" },
      { id: "configuration", label: "Configuration", icon: "lucide:sliders-horizontal" },
      { id: "personalization", label: "Personalization", icon: "lucide:smile" },
      { id: "shortcuts", label: "Keyboard shortcuts", icon: "lucide:keyboard" },
      { id: "billing", label: "Usage & billing", icon: "lucide:gauge" },
    ],
  },
  {
    label: "Workspace",
    items: [
      { id: "team", label: "Team members", icon: "lucide:users" },
    ],
  },
  {
    label: "Integrations",
    items: [
      { id: "appshots", label: "Appshots", icon: "lucide:scan" },
      { id: "mcp", label: "MCP servers", icon: "lucide:paperclip" },
      { id: "browser", label: "Browser", icon: "lucide:app-window" },
      { id: "computer-use", label: "Computer use", icon: "lucide:mouse-pointer-click" },
    ],
  },
  {
    label: "Coding",
    items: [
      { id: "hooks", label: "Hooks", icon: "lucide:anchor" },
      { id: "connections", label: "Connections", icon: "lucide:globe" },
      { id: "git", label: "Git", icon: "lucide:git-branch" },
      { id: "environments", label: "Environments", icon: "lucide:monitor" },
      { id: "worktrees", label: "Worktrees", icon: "lucide:git-merge" },
    ],
  },
  {
    label: "Archived",
    items: [
      { id: "archived-chats", label: "Archived chats", icon: "lucide:archive" },
    ],
  },
]

export const NAV_ITEMS: SettingsNavItem[] = NAV_SECTIONS.flatMap(
  (section) => section.items
)

export function settingsHref(id: string) {
  return `/w/settings/${id}`
}

export function findNavItem(id: string) {
  return NAV_ITEMS.find((item) => item.id === id)
}
