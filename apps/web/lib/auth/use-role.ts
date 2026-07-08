"use client"

import { useAuth } from "./auth-context"

// The organization roles, from most to least privileged. `owner ⊃ admin ⊃
// member` — an owner can do everything an admin can, an admin everything a
// member can. These mirror the backend authorization tiers; the backend is
// still the source of truth, the helpers here only gate the UI.
export type OrgRole = "owner" | "admin" | "member"

// isOwnerRole reports whether the role owns the org. Owner-only mutations
// (transfer ownership, delete org, granting/revoking the owner role) gate on
// this.
export function isOwnerRole(role: string | null | undefined): boolean {
  return role === "owner"
}

// isAdminRole reports whether the role can administer the org (owner or admin).
// Admin-gated surfaces (teams, plugin/knowledge provisioning, connections,
// member management) gate on this.
export function isAdminRole(role: string | null | undefined): boolean {
  return role === "owner" || role === "admin"
}

// useRole returns the caller's role in the active org, or null when unknown.
export function useRole(): string | null {
  const { activeOrg } = useAuth()
  return activeOrg?.role ?? null
}

// useIsAdmin reports whether the caller can administer the active org.
export function useIsAdmin(): boolean {
  return isAdminRole(useRole())
}

// useIsOwner reports whether the caller owns the active org.
export function useIsOwner(): boolean {
  return isOwnerRole(useRole())
}
