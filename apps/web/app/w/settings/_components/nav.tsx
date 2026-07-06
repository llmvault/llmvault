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
      { id: "general", label: "General", icon: "settings" },
      { id: "appearance", label: "Appearance", icon: "sun" },
      { id: "billing", label: "Usage & billing", icon: "gauge" },
      { id: "archived", label: "Archived chats", icon: "archive" },
    ],
  },
  {
    label: "Workspace",
    items: [
      { id: "agents", label: "Agents", icon: "bot" },
      { id: "teams", label: "Teams", icon: "users" },
      { id: "channels", label: "Channels", icon: "hash" },
      { id: "memories", label: "Memories", icon: "brain" },
      { id: "knowledge", label: "Knowledge", icon: "folder-open" },
    ],
  },
  {
    label: "Coding",
    items: [
      { id: "environments", label: "Environments", icon: "monitor" },
    ],
  },
]

export function settingsHref(id: string) {
  return `/w/settings/${id}`
}
