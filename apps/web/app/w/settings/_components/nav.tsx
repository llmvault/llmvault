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
      { id: "appearance", label: "Appearance", icon: "lucide:sun" },
      { id: "billing", label: "Usage & billing", icon: "lucide:gauge" },
    ],
  },
  {
    label: "Workspace",
    items: [
      { id: "agents", label: "Agents", icon: "lucide:bot" },
      { id: "teams", label: "Teams", icon: "lucide:users" },
    ],
  },
  {
    label: "Coding",
    items: [
      { id: "environments", label: "Environments", icon: "lucide:monitor" },
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
