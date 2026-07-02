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
]

export function settingsHref(id: string) {
  return `/w/settings/${id}`
}
