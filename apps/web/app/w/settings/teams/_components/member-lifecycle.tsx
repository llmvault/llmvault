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
import {
  assignableRoles,
  canChangeRole,
  canRemoveMember,
} from "./member-actions"
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
  const [removeOpen, setRemoveOpen] = useState(false)

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
    removeMember.mutate(
      { params: { path: { userID: member.user_id } } },
      {
        onSuccess: () => {
          toast.success(`Removed ${memberLabel(member)}`)
          setRemoveOpen(false)
          queryClient.invalidateQueries({ queryKey: MEMBERS_KEY })
        },
        onError: (err) => {
          toast.danger(extractErrorMessage(err, "Could not remove member"))
          setRemoveOpen(false)
        },
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
          onPress={() => setRemoveOpen(true)}
        >
          {removeMember.isPending ? (
            <Spinner color="current" size="sm" />
          ) : null}
          Remove
        </Button>
      ) : null}

      <ConfirmDialog
        open={removeOpen}
        pending={removeMember.isPending}
        heading={`Remove ${memberLabel(member)} from this workspace?`}
        description="They lose access to this workspace immediately. You can invite them again later."
        confirmLabel="Remove member"
        icon="user-minus"
        onOpenChange={setRemoveOpen}
        onConfirm={handleRemove}
      />
    </div>
  )
}
