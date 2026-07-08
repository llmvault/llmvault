"use client"

import { useMemo, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Chip, ListBox, Select, Spinner, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { cn } from "@/lib/utils"
import {
  EmptyRow,
  MEMBERS_KEY,
  RowSkeleton,
  initials,
  memberLabel,
  roleLabel,
  type Member,
  type TeamMember,
} from "../_components/team-settings"

export function TeamMembersSection({
  teamId,
  members,
  orgMembers,
  isLoading,
  onInvite,
  onChanged,
  readOnly,
}: {
  teamId: string
  members: TeamMember[]
  orgMembers: Member[]
  isLoading: boolean
  onInvite: () => void
  onChanged: () => void
  readOnly?: boolean
}) {
  const queryClient = useQueryClient()
  const addMember = $api.useMutation(
    "put",
    "/v1/orgs/current/teams/{id}/members/{userID}"
  )
  const removeMember = $api.useMutation(
    "delete",
    "/v1/orgs/current/teams/{id}/members/{userID}"
  )
  const [selectedUserId, setSelectedUserId] = useState("")
  const memberIDs = useMemo(
    () => new Set(members.map((member) => member.user_id).filter(Boolean)),
    [members]
  )
  const candidates = useMemo(
    () =>
      orgMembers.filter(
        (member) => member.user_id && !memberIDs.has(member.user_id)
      ),
    [memberIDs, orgMembers]
  )
  const selectedCandidate = candidates.find(
    (member) => member.user_id === selectedUserId
  )

  function handleAddMember() {
    if (!selectedUserId || addMember.isPending) return
    addMember.mutate(
      {
        params: { path: { id: teamId, userID: selectedUserId } },
        body: { role: "member" },
      },
      {
        onSuccess: () => {
          toast.success("Member added")
          setSelectedUserId("")
          queryClient.invalidateQueries({ queryKey: MEMBERS_KEY })
          onChanged()
        },
        onError: (err) =>
          toast.danger(extractErrorMessage(err, "Could not add member")),
      }
    )
  }

  function handleRemoveMember(member: TeamMember) {
    if (!member.user_id || removeMember.isPending) return
    removeMember.mutate(
      { params: { path: { id: teamId, userID: member.user_id } } },
      {
        onSuccess: () => {
          toast.success("Member removed")
          onChanged()
        },
        onError: (err) =>
          toast.danger(extractErrorMessage(err, "Could not remove member")),
      }
    )
  }

  return (
    <section className="flex flex-col gap-3">
      <div className="flex items-center justify-between gap-4">
        <h2 className="text-sm font-medium">
          Members{members.length ? ` (${members.length})` : ""}
        </h2>
        {readOnly ? null : (
          <Button variant="tertiary" size="sm" onPress={onInvite}>
            <AppIcon icon="user-plus" className="h-4 w-4" />
            Invite to team
          </Button>
        )}
      </div>

      {readOnly ? null : (
        <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
          <label className="flex min-w-0 flex-1 flex-col gap-1.5">
            <span className="text-sm font-medium">Add existing member</span>
            <Select
              aria-label="Add existing member"
              selectedKey={selectedUserId || null}
              onSelectionChange={(key) =>
                setSelectedUserId(key === null ? "" : String(key))
              }
              isDisabled={addMember.isPending || candidates.length === 0}
              className="w-full"
            >
              <Select.Trigger className="h-9 w-full justify-between px-3 text-sm transition-colors">
                {selectedCandidate ? (
                  <span className="truncate">
                    {memberLabel(selectedCandidate)}
                  </span>
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
            variant="primary"
            size="sm"
            isDisabled={!selectedUserId || addMember.isPending}
            onPress={handleAddMember}
          >
            {addMember.isPending ? <Spinner color="current" size="sm" /> : null}
            Add
          </Button>
        </div>
      )}

      <div className="overflow-hidden rounded-2xl border border-border bg-surface">
        {isLoading ? (
          <RowSkeleton />
        ) : members.length === 0 ? (
          <EmptyRow text="No members in this team yet." />
        ) : (
          members.map((member, index) => (
            <TeamMemberRow
              key={member.user_id ?? index}
              member={member}
              last={index === members.length - 1}
              isBusy={removeMember.isPending}
              onRemove={() => handleRemoveMember(member)}
              readOnly={readOnly}
            />
          ))
        )}
      </div>
    </section>
  )
}

function TeamMemberRow({
  member,
  last,
  isBusy,
  onRemove,
  readOnly,
}: {
  member: TeamMember
  last?: boolean
  isBusy: boolean
  onRemove: () => void
  readOnly?: boolean
}) {
  return (
    <div
      className={cn(
        "flex items-center gap-3 px-4 py-3.5",
        last ? "" : "border-b border-border"
      )}
    >
      <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-default text-xs font-medium text-muted">
        {initials(member.name, member.email)}
      </div>
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="truncate text-sm font-medium">
          {memberLabel(member)}
        </span>
        <span className="truncate text-sm text-muted">{member.email}</span>
      </div>
      <Chip size="sm">{roleLabel(member.role)}</Chip>
      {readOnly ? null : (
        <Button variant="ghost" size="sm" isDisabled={isBusy} onPress={onRemove}>
          Remove
        </Button>
      )}
    </div>
  )
}
