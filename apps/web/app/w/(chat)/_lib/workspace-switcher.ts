interface WorkspaceSwitcherUser {
  email?: string
  name?: string
}

export function userPrimaryLabel(user: WorkspaceSwitcherUser | null) {
  return user?.name?.trim() || user?.email?.trim() || "Signed in"
}

export function userSecondaryLabel(user: WorkspaceSwitcherUser | null) {
  const name = user?.name?.trim()
  const email = user?.email?.trim()
  return name && email ? email : null
}

export function labelInitials(label: string) {
  const parts = label.trim().split(/\s+/).filter(Boolean)
  if (parts.length >= 2) {
    return `${parts[0][0]}${parts[1][0]}`.toUpperCase()
  }
  return (parts[0] ?? "?").slice(0, 2).toUpperCase()
}

export function normalizedWorkspaceName(name: string) {
  const trimmed = name.trim()
  return trimmed || null
}
