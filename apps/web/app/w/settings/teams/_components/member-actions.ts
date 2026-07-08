import { isAdminRole, isOwnerRole } from "@/lib/auth/use-role"

// Pure gating helpers for the org members list. These mirror (but do not
// replace) the backend's own authorization checks in
// internal/handler/orgs_members.go — the backend is still the source of
// truth, these only decide what controls to render.

// assignableRoles lists the roles the actor is allowed to grant. An owner
// can grant any tier up to and including owner; an admin can grant admin or
// member, but never owner (only an owner may create another owner).
export function assignableRoles(actorRole: string | null | undefined): string[] {
  if (isOwnerRole(actorRole)) return ["owner", "admin", "member"]
  if (isAdminRole(actorRole)) return ["admin", "member"]
  return []
}

// canChangeRole reports whether the actor may change the target member's
// role at all (the actual new role is further constrained by
// assignableRoles). Blocked when: the actor isn't an admin/owner, the actor
// is targeting themselves, or the target is an owner and either the actor
// isn't an owner or the target is the org's last remaining owner.
export function canChangeRole(args: {
  actorRole?: string | null
  targetRole?: string | null
  isSelf: boolean
  ownerCount: number
}): boolean {
  if (!isAdminRole(args.actorRole)) return false
  if (args.isSelf) return false
  if (args.targetRole !== "owner") return true
  return isOwnerRole(args.actorRole) && args.ownerCount > 1
}

// canRemoveMember mirrors canChangeRole's shape: admins may remove
// non-owners, only owners may remove owners, and the last owner can never
// be removed.
export function canRemoveMember(args: {
  actorRole?: string | null
  targetRole?: string | null
  isSelf: boolean
  ownerCount: number
}): boolean {
  if (!isAdminRole(args.actorRole)) return false
  if (args.isSelf) return false
  if (args.targetRole !== "owner") return true
  return isOwnerRole(args.actorRole) && args.ownerCount > 1
}

// canTransferOwnership reports whether the actor may transfer org
// ownership to another member. Owner-only, and the actor can't transfer to
// themselves.
export function canTransferOwnership(args: {
  actorRole?: string | null
  isSelf: boolean
}): boolean {
  return isOwnerRole(args.actorRole) && !args.isSelf
}

// canDeleteOrg reports whether the actor may permanently delete the org.
// Owner-only.
export function canDeleteOrg(actorRole?: string | null): boolean {
  return isOwnerRole(actorRole)
}

// ownerCount counts how many members currently hold the "owner" role.
// Used to guard the last-owner-can't-be-demoted-or-removed rule.
export function ownerCount(members: { role?: string }[]): number {
  return members.filter((member) => member.role === "owner").length
}
