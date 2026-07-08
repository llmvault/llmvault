"use client"

import { useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import {
  Avatar,
  Button,
  Chip,
  ListBox,
  Select,
  Spinner,
  toast,
} from "@heroui/react"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { cn } from "@/lib/utils"
import { assignableRoles, canChangeRole, canRemoveMember } from "./member-actions"
import {
  MEMBERS_KEY,
  initials,
  memberLabel,
  roleLabel,
  type Member,
} from "./team-settings"

// OrgMemberRow renders a single org member with, when the acting caller is
// allowed to, an inline role switcher and a "Remove" action. The backend is
// still the source of truth for every mutation here — these gates only
// decide what controls render; a 403/422 from the API surfaces as a toast.
export function OrgMemberRow({
  member,
  isYou,
  actorRole,
  ownerCount,
  last,
}: {
  member: Member
  isYou: boolean
  actorRole: string | null | undefined
  ownerCount: number
  last?: boolean
}) {
  const queryClient = useQueryClient()
  const changeRole = $api.useMutation(
    "patch",
    "/v1/orgs/current/members/{userID}/role"
  )
  const removeMember = $api.useMutation(
    "delete",
    "/v1/orgs/current/members/{userID}"
  )

  const roleOptions = assignableRoles(actorRole)
  const showRoleSelect =
    roleOptions.length > 0 &&
    canChangeRole({
      actorRole,
      targetRole: member.role,
      isSelf: isYou,
      ownerCount,
    })
  const showRemove = canRemoveMember({
    actorRole,
    targetRole: member.role,
    isSelf: isYou,
    ownerCount,
  })

  function handleRoleChange(role: string) {
    if (!member.user_id || role === member.role || changeRole.isPending) {
      return
    }
    changeRole.mutate(
      { params: { path: { userID: member.user_id } }, body: { role } },
      {
        onSuccess: () => {
          toast.success(`${memberLabel(member)} is now ${roleLabel(role)}`)
          queryClient.invalidateQueries({ queryKey: MEMBERS_KEY })
        },
        onError: (err) =>
          toast.danger(extractErrorMessage(err, "Could not update role")),
      }
    )
  }

  function handleRemove() {
    if (!member.user_id || removeMember.isPending) return
    const confirmed = window.confirm(
      `Remove ${memberLabel(member)} from this workspace?`
    )
    if (!confirmed) return
    removeMember.mutate(
      { params: { path: { userID: member.user_id } } },
      {
        onSuccess: () => {
          toast.success(`Removed ${memberLabel(member)}`)
          queryClient.invalidateQueries({ queryKey: MEMBERS_KEY })
        },
        onError: (err) =>
          toast.danger(extractErrorMessage(err, "Could not remove member")),
      }
    )
  }

  return (
    <div
      className={cn(
        "flex items-center gap-3 px-4 py-3.5",
        last ? "" : "border-b border-border"
      )}
    >
      <Avatar size="sm" className="shrink-0">
        <Avatar.Fallback>{initials(member.name, member.email)}</Avatar.Fallback>
      </Avatar>
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="truncate text-sm font-medium">
          {memberLabel(member)}
          {isYou ? <span className="text-muted"> (You)</span> : null}
        </span>
        <span className="truncate text-sm text-muted">{member.email}</span>
      </div>

      {showRoleSelect ? (
        <Select
          aria-label={`Change role for ${memberLabel(member)}`}
          selectedKey={member.role ?? null}
          onSelectionChange={(key) => {
            if (key !== null) handleRoleChange(String(key))
          }}
          isDisabled={changeRole.isPending}
          className="w-32 shrink-0"
        >
          <Select.Trigger className="h-8 w-full justify-between px-2.5 text-sm transition-colors">
            <span className="truncate">{roleLabel(member.role)}</span>
            {changeRole.isPending ? (
              <Spinner color="current" size="sm" />
            ) : (
              <Select.Indicator />
            )}
          </Select.Trigger>
          <Select.Popover className="p-1.5">
            <ListBox>
              {roleOptions.map((role) => (
                <ListBox.Item key={role} id={role} textValue={roleLabel(role)}>
                  {roleLabel(role)}
                </ListBox.Item>
              ))}
            </ListBox>
          </Select.Popover>
        </Select>
      ) : (
        <Chip size="sm" color={member.role === "owner" ? "accent" : "default"}>
          {roleLabel(member.role)}
        </Chip>
      )}

      {showRemove ? (
        <Button
          variant="danger-soft"
          size="sm"
          isDisabled={removeMember.isPending}
          onPress={handleRemove}
        >
          {removeMember.isPending ? (
            <Spinner color="current" size="sm" />
          ) : null}
          Remove
        </Button>
      ) : null}
    </div>
  )
}

// OrgDangerZone surfaces owner-only, org-wide destructive actions: transfer
// ownership to another member, and permanently delete the workspace. Render
// this only for callers who hold the owner role — the backend enforces it
// too, but there's no reason to show controls that will always 403.
export function OrgDangerZone({
  members,
  currentUserId,
}: {
  members: Member[]
  currentUserId: string | undefined
}) {
  const queryClient = useQueryClient()
  const transferOwnership = $api.useMutation(
    "post",
    "/v1/orgs/current/transfer-ownership"
  )
  const deleteOrg = $api.useMutation("delete", "/v1/orgs/current")
  const [transferTargetId, setTransferTargetId] = useState("")
  const [deleteOpen, setDeleteOpen] = useState(false)

  // The parent only renders OrgDangerZone for owners (canTransferOwnership
  // requires actorRole === "owner" and isSelf === false), so the remaining
  // gate here is just excluding the caller from their own transfer target
  // list.
  const candidates = members.filter(
    (member) => member.user_id && member.user_id !== currentUserId
  )
  const transferTarget = candidates.find(
    (member) => member.user_id === transferTargetId
  )

  function handleTransfer() {
    if (!transferTarget?.user_id || transferOwnership.isPending) return
    const confirmed = window.confirm(
      `Transfer workspace ownership to ${memberLabel(transferTarget)}? You will become an admin.`
    )
    if (!confirmed) return
    transferOwnership.mutate(
      { body: { new_owner_user_id: transferTarget.user_id } },
      {
        onSuccess: () => {
          toast.success(`Ownership transferred to ${memberLabel(transferTarget)}`)
          setTransferTargetId("")
          queryClient.invalidateQueries({ queryKey: MEMBERS_KEY })
        },
        onError: (err) =>
          toast.danger(
            extractErrorMessage(err, "Could not transfer ownership")
          ),
      }
    )
  }

  function handleDelete() {
    deleteOrg.mutate(undefined, {
      onSuccess: () => {
        toast.success("Workspace deleted")
        setDeleteOpen(false)
        window.location.href = "/"
      },
      onError: (err) => {
        toast.danger(extractErrorMessage(err, "Could not delete workspace"))
        setDeleteOpen(false)
      },
    })
  }

  return (
    <section className="flex flex-col gap-3">
      <div>
        <h2 className="text-sm font-medium text-danger">Danger zone</h2>
        <p className="mt-1 text-sm text-muted">
          Owner-only actions that affect the entire workspace.
        </p>
      </div>

      <div className="bg-surface flex flex-col gap-6 rounded-2xl border border-danger/30 p-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
          <label className="flex min-w-0 flex-1 flex-col gap-1.5">
            <span className="text-sm font-medium">Transfer ownership</span>
            <Select
              aria-label="Transfer ownership to"
              selectedKey={transferTargetId || null}
              onSelectionChange={(key) =>
                setTransferTargetId(key === null ? "" : String(key))
              }
              isDisabled={transferOwnership.isPending || candidates.length === 0}
              className="w-full"
            >
              <Select.Trigger className="h-9 w-full justify-between px-3 text-sm transition-colors">
                {transferTarget ? (
                  <span className="truncate">{memberLabel(transferTarget)}</span>
                ) : (
                  <Select.Value />
                )}
                <Select.Indicator />
              </Select.Trigger>
              <Select.Popover className="p-1.5">
                <ListBox>
                  {candidates.map((member) => (
                    <ListBox.Item
                      key={member.user_id}
                      id={member.user_id}
                      textValue={memberLabel(member)}
                    >
                      <span className="flex min-w-0 flex-col">
                        <span className="truncate text-sm">
                          {memberLabel(member)}
                        </span>
                        <span className="truncate text-xs text-muted">
                          {member.email}
                        </span>
                      </span>
                    </ListBox.Item>
                  ))}
                </ListBox>
              </Select.Popover>
            </Select>
          </label>
          <Button
            variant="danger-soft"
            size="sm"
            isDisabled={!transferTargetId || transferOwnership.isPending}
            onPress={handleTransfer}
          >
            {transferOwnership.isPending ? (
              <Spinner color="current" size="sm" />
            ) : null}
            Transfer
          </Button>
        </div>

        <div className="flex items-center justify-between gap-4 border-t border-danger/20 pt-4">
          <div className="min-w-0">
            <p className="text-sm font-medium">Delete workspace</p>
            <p className="text-sm text-muted">
              Permanently deletes this workspace and all of its data. This
              cannot be undone.
            </p>
          </div>
          <Button
            variant="danger-soft"
            size="sm"
            className="shrink-0"
            onPress={() => setDeleteOpen(true)}
          >
            Delete workspace
          </Button>
        </div>
      </div>

      <ConfirmDialog
        open={deleteOpen}
        pending={deleteOrg.isPending}
        heading="Delete this workspace?"
        description="This permanently deletes the workspace and all of its agents, channels, sessions, and data. This cannot be undone."
        confirmLabel="Delete workspace"
        icon="trash-2"
        onOpenChange={setDeleteOpen}
        onConfirm={handleDelete}
      />
    </section>
  )
}
