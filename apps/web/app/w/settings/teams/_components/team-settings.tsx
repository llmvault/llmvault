"use client"

import { useMemo, useState } from "react"
import NextLink from "next/link"
import { useQueryClient } from "@tanstack/react-query"
import {
  Avatar,
  Button,
  Chip,
  Input,
  ListBox,
  Modal,
  Select,
  Spinner,
  toast,
} from "@heroui/react"
import { Icon } from "@iconify/react"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { cn } from "@/lib/utils"
import type { components } from "@/lib/api/schema"

export type Team = components["schemas"]["teamResponse"]
export type Member = components["schemas"]["orgMemberResponse"]
export type Invite = components["schemas"]["orgInviteResponse"]
export type TeamMember = components["schemas"]["teamMemberResponse"]
export type Channel = components["schemas"]["channelResponse"]

export const TEAMS_KEY = ["get", "/v1/orgs/current/teams"] as const
export const MEMBERS_KEY = ["get", "/v1/orgs/current/members"] as const
export const INVITES_KEY = ["get", "/v1/orgs/current/invites"] as const
export const CHANNELS_KEY = ["get", "/v1/channels"] as const

const ROLE_OPTIONS = [
  { value: "admin", label: "Admin" },
  { value: "member", label: "Member" },
  { value: "viewer", label: "Viewer" },
]

export function roleLabel(role: string | undefined) {
  if (!role) return "Member"
  return role.charAt(0).toUpperCase() + role.slice(1)
}

export function initials(name: string | undefined, email: string | undefined) {
  const source = (name && name.trim()) || email || "?"
  const parts = source.split(/\s+/).filter(Boolean)
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase()
  return source.slice(0, 2).toUpperCase()
}

export function formatDate(value: string | undefined | null) {
  if (!value) return "Not set"
  return new Intl.DateTimeFormat("en-NG", {
    day: "numeric",
    month: "short",
    year: "numeric",
  }).format(new Date(value))
}

export function teamLabel(team: Team | undefined) {
  return team?.name?.trim() || "Untitled team"
}

export function memberLabel(member: Member | TeamMember | undefined) {
  return member?.name?.trim() || member?.email?.trim() || "Unknown member"
}

export function channelLabel(channel: Channel | undefined) {
  return channel?.name?.trim() || "Untitled channel"
}

export function teamMap(teams: Team[]) {
  return new Map(teams.flatMap((team) => (team.id ? [[team.id, team]] : [])))
}

export function TeamRow({ team, last }: { team: Team; last?: boolean }) {
  const href = team.id ? `/w/settings/teams/${team.id}` : "/w/settings/teams"

  return (
    <NextLink
      href={href}
      className={cn(
        "group hover:bg-default flex items-center gap-3 px-4 py-3.5 transition-colors",
        last ? "" : "border-b border-border"
      )}
    >
      <span className="bg-default flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-muted-foreground">
        <Icon icon="lucide:users-round" className="h-4 w-4" />
      </span>
      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="truncate text-sm font-medium text-foreground">
          {teamLabel(team)}
        </span>
        <span className="truncate text-sm text-muted">
          {team.description || "No description"}
        </span>
      </span>
      <span className="hidden shrink-0 items-center gap-2 text-xs text-muted sm:flex">
        <span>{team.member_count ?? 0} members</span>
        <span>{team.channel_count ?? 0} channels</span>
      </span>
      <Icon
        icon="lucide:chevron-right"
        className="h-4 w-4 shrink-0 text-muted transition-colors group-hover:text-foreground"
      />
    </NextLink>
  )
}

export function MemberRow({
  member,
  isYou,
  last,
}: {
  member: Member
  isYou: boolean
  last?: boolean
}) {
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
      <Chip size="sm" color={member.role === "owner" ? "accent" : "default"}>
        {roleLabel(member.role)}
      </Chip>
    </div>
  )
}

export function InviteRow({
  invite,
  teamsById,
  last,
}: {
  invite: Invite
  teamsById: Map<string, Team>
  last?: boolean
}) {
  const queryClient = useQueryClient()
  const revoke = $api.useMutation("delete", "/v1/orgs/current/invites/{id}")
  const resend = $api.useMutation(
    "post",
    "/v1/orgs/current/invites/{id}/resend"
  )
  const busy = revoke.isPending || resend.isPending
  const inviteTeamIDs = invite.team_ids ?? []

  function handleResend() {
    if (!invite.id) return
    resend.mutate(
      { params: { path: { id: invite.id } } },
      {
        onSuccess: () => toast.success(`Invitation resent to ${invite.email}`),
        onError: (err) =>
          toast.danger(extractErrorMessage(err, "Could not resend invitation")),
      }
    )
  }

  function handleRevoke() {
    if (!invite.id) return
    revoke.mutate(
      { params: { path: { id: invite.id } } },
      {
        onSuccess: () => {
          toast.success(`Revoked invitation for ${invite.email}`)
          queryClient.invalidateQueries({ queryKey: INVITES_KEY })
        },
        onError: (err) =>
          toast.danger(extractErrorMessage(err, "Could not revoke invitation")),
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
        <Avatar.Fallback>
          <Icon icon="lucide:mail" className="h-4 w-4" />
        </Avatar.Fallback>
      </Avatar>
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <span className="truncate text-sm font-medium">{invite.email}</span>
        <span className="truncate text-sm text-muted">
          Invited {formatDate(invite.created_at)}. Expires{" "}
          {formatDate(invite.expires_at)}
        </span>
        {inviteTeamIDs.length > 0 ? (
          <span className="flex flex-wrap gap-1">
            {inviteTeamIDs.map((teamID) => (
              <Chip key={teamID} size="sm">
                {teamLabel(teamsById.get(teamID))}
              </Chip>
            ))}
          </span>
        ) : null}
      </div>
      <Chip size="sm">{roleLabel(invite.role)}</Chip>
      <div className="flex shrink-0 items-center gap-1">
        <Button
          variant="ghost"
          size="sm"
          isDisabled={busy}
          onPress={handleResend}
        >
          {resend.isPending ? <Spinner color="current" size="sm" /> : null}
          Resend
        </Button>
        <Button
          variant="danger-soft"
          size="sm"
          isDisabled={busy}
          onPress={handleRevoke}
        >
          {revoke.isPending ? <Spinner color="current" size="sm" /> : null}
          Revoke
        </Button>
      </div>
    </div>
  )
}

export function InviteMemberModal({
  open,
  onOpenChange,
  orgName,
  teams,
  defaultTeamIds = [],
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  orgName: string | undefined
  teams: Team[]
  defaultTeamIds?: string[]
}) {
  const queryClient = useQueryClient()
  const createInvite = $api.useMutation("post", "/v1/orgs/current/invites")
  const [email, setEmail] = useState("")
  const [role, setRole] = useState("member")
  const [selectedTeamIds, setSelectedTeamIds] =
    useState<string[]>(defaultTeamIds)

  function close() {
    setEmail("")
    setRole("member")
    setSelectedTeamIds(defaultTeamIds)
    onOpenChange(false)
  }

  function toggleTeam(teamID: string) {
    setSelectedTeamIds((current) =>
      current.includes(teamID)
        ? current.filter((id) => id !== teamID)
        : [...current, teamID]
    )
  }

  function handleSubmit() {
    const trimmed = email.trim()
    if (!trimmed) {
      toast.danger("Enter an email address")
      return
    }
    createInvite.mutate(
      {
        body: {
          email: trimmed,
          role,
          team_ids: selectedTeamIds.length > 0 ? selectedTeamIds : undefined,
        },
      },
      {
        onSuccess: () => {
          toast.success(`Invitation sent to ${trimmed}`)
          queryClient.invalidateQueries({ queryKey: INVITES_KEY })
          queryClient.invalidateQueries({ queryKey: MEMBERS_KEY })
          queryClient.invalidateQueries({ queryKey: TEAMS_KEY })
          close()
        },
        onError: (err) =>
          toast.danger(extractErrorMessage(err, "Could not send invitation")),
      }
    )
  }

  return (
    <Modal
      isOpen={open}
      onOpenChange={(next) => {
        if (!next) {
          if (!createInvite.isPending) close()
        } else {
          onOpenChange(true)
        }
      }}
    >
      <Modal.Backdrop className="bg-background/80 backdrop-blur-sm">
        <Modal.Container placement="center" size="md">
          <Modal.Dialog className="p-8">
            <Modal.CloseTrigger />

            <Modal.Header>
              <Modal.Icon className="bg-default size-12 text-foreground">
                <Icon icon="lucide:user-plus" className="h-6 w-6" />
              </Modal.Icon>
              <div className="flex flex-col gap-1">
                <Modal.Heading>Invite a member</Modal.Heading>
                <p className="text-sm text-muted">
                  They&apos;ll get an email invitation to join{" "}
                  {orgName ?? "your workspace"}.
                </p>
              </div>
            </Modal.Header>

            <Modal.Body>
              <div className="flex flex-col gap-4">
                <label className="flex flex-col gap-1.5">
                  <span className="text-sm font-medium">Email address</span>
                  <Input
                    type="email"
                    value={email}
                    onChange={(event) => setEmail(event.target.value)}
                    placeholder="teammate@company.com"
                    className="w-full"
                    autoFocus
                  />
                </label>
                <label className="flex flex-col gap-1.5">
                  <span className="text-sm font-medium">Role</span>
                  <RoleSelect value={role} onChange={setRole} />
                </label>
                <div className="flex flex-col gap-2">
                  <span className="text-sm font-medium">Teams</span>
                  <TeamPicker
                    teams={teams}
                    selectedTeamIds={selectedTeamIds}
                    onToggle={toggleTeam}
                    isDisabled={createInvite.isPending}
                  />
                </div>
              </div>
            </Modal.Body>

            <Modal.Footer>
              <Button
                variant="tertiary"
                size="sm"
                isDisabled={createInvite.isPending}
                onPress={close}
              >
                Cancel
              </Button>
              <Button
                variant="primary"
                size="sm"
                isDisabled={createInvite.isPending || !email.trim()}
                onPress={handleSubmit}
              >
                {createInvite.isPending ? (
                  <Spinner color="current" size="sm" />
                ) : null}
                Send invitation
              </Button>
            </Modal.Footer>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}

export function TeamFormModal({
  open,
  onOpenChange,
  initialTeam,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  initialTeam?: Team
}) {
  const queryClient = useQueryClient()
  const createTeam = $api.useMutation("post", "/v1/orgs/current/teams")
  const updateTeam = $api.useMutation("patch", "/v1/orgs/current/teams/{id}")
  const editing = Boolean(initialTeam?.id)
  const busy = createTeam.isPending || updateTeam.isPending
  const [name, setName] = useState(initialTeam?.name ?? "")
  const [description, setDescription] = useState(initialTeam?.description ?? "")

  function close() {
    if (busy) return
    setName(initialTeam?.name ?? "")
    setDescription(initialTeam?.description ?? "")
    onOpenChange(false)
  }

  function handleSubmit() {
    const trimmed = name.trim()
    if (!trimmed) {
      toast.danger("Enter a team name")
      return
    }
    const body = {
      name: trimmed,
      description: description.trim(),
    }

    if (editing && initialTeam?.id) {
      updateTeam.mutate(
        { params: { path: { id: initialTeam.id } }, body },
        {
          onSuccess: () => {
            toast.success("Team updated")
            queryClient.invalidateQueries({ queryKey: TEAMS_KEY })
            queryClient.invalidateQueries({
              queryKey: ["get", "/v1/orgs/current/teams/{id}"],
            })
            close()
          },
          onError: (err) =>
            toast.danger(extractErrorMessage(err, "Could not update team")),
        }
      )
      return
    }

    createTeam.mutate(
      { body },
      {
        onSuccess: () => {
          toast.success("Team created")
          queryClient.invalidateQueries({ queryKey: TEAMS_KEY })
          close()
        },
        onError: (err) =>
          toast.danger(extractErrorMessage(err, "Could not create team")),
      }
    )
  }

  return (
    <Modal
      isOpen={open}
      onOpenChange={(next) => {
        if (!next) close()
        else onOpenChange(true)
      }}
    >
      <Modal.Backdrop className="bg-background/80 backdrop-blur-sm">
        <Modal.Container placement="center" size="md">
          <Modal.Dialog className="p-8">
            <Modal.CloseTrigger />
            <Modal.Header>
              <Modal.Icon className="bg-default size-12 text-foreground">
                <Icon icon="lucide:users-round" className="h-6 w-6" />
              </Modal.Icon>
              <div className="flex flex-col gap-1">
                <Modal.Heading>
                  {editing ? "Edit team" : "New team"}
                </Modal.Heading>
                <p className="text-sm text-muted">
                  Teams group members and restrict assigned channels.
                </p>
              </div>
            </Modal.Header>
            <Modal.Body>
              <div className="flex flex-col gap-4">
                <label className="flex flex-col gap-1.5">
                  <span className="text-sm font-medium">Name</span>
                  <Input
                    value={name}
                    onChange={(event) => setName(event.target.value)}
                    placeholder="Engineering"
                    className="w-full"
                    autoFocus
                  />
                </label>
                <label className="flex flex-col gap-1.5">
                  <span className="text-sm font-medium">Description</span>
                  <Input
                    value={description}
                    onChange={(event) => setDescription(event.target.value)}
                    placeholder="People and channels for this group"
                    className="w-full"
                  />
                </label>
              </div>
            </Modal.Body>
            <Modal.Footer>
              <Button
                variant="tertiary"
                size="sm"
                isDisabled={busy}
                onPress={close}
              >
                Cancel
              </Button>
              <Button
                variant="primary"
                size="sm"
                isDisabled={busy || !name.trim()}
                onPress={handleSubmit}
              >
                {busy ? <Spinner color="current" size="sm" /> : null}
                {editing ? "Save changes" : "Create team"}
              </Button>
            </Modal.Footer>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}

function RoleSelect({
  value,
  onChange,
}: {
  value: string
  onChange: (value: string) => void
}) {
  return (
    <Select
      aria-label="Role"
      selectedKey={value}
      onSelectionChange={(key) => onChange(String(key))}
      className="w-full"
    >
      <Select.Trigger className="w-full justify-between">
        <Select.Value />
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover>
        <ListBox>
          {ROLE_OPTIONS.map((option) => (
            <ListBox.Item key={option.value} id={option.value}>
              {option.label}
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}

function TeamPicker({
  teams,
  selectedTeamIds,
  onToggle,
  isDisabled,
}: {
  teams: Team[]
  selectedTeamIds: string[]
  onToggle: (teamID: string) => void
  isDisabled: boolean
}) {
  const selectableTeams = useMemo(
    () => teams.filter((team) => Boolean(team.id)),
    [teams]
  )

  if (selectableTeams.length === 0) {
    return (
      <div className="bg-field-background rounded-xl border border-border px-3 py-3 text-sm text-muted">
        No teams yet.
      </div>
    )
  }

  return (
    <div className="bg-field-background flex flex-col overflow-hidden rounded-xl border border-border">
      {selectableTeams.map((team, index) => {
        const selected = Boolean(team.id && selectedTeamIds.includes(team.id))
        return (
          <button
            key={team.id}
            type="button"
            disabled={isDisabled}
            onClick={() => {
              if (team.id) onToggle(team.id)
            }}
            className={cn(
              "hover:bg-default flex items-center gap-3 px-3 py-2 text-left text-sm transition-colors disabled:cursor-not-allowed disabled:opacity-60",
              index === selectableTeams.length - 1
                ? ""
                : "border-b border-border"
            )}
          >
            <span
              className={cn(
                "flex h-4 w-4 shrink-0 items-center justify-center rounded border",
                selected
                  ? "border-accent bg-accent text-accent-foreground"
                  : "border-border"
              )}
            >
              {selected ? (
                <Icon icon="lucide:check" className="h-3 w-3" />
              ) : null}
            </span>
            <span className="min-w-0 flex-1 truncate">{teamLabel(team)}</span>
            <span className="shrink-0 text-xs text-muted">
              {team.member_count ?? 0} members
            </span>
          </button>
        )
      })}
    </div>
  )
}

export function EmptyRow({ text }: { text: string }) {
  return <div className="px-4 py-8 text-center text-sm text-muted">{text}</div>
}

export function RowSkeleton() {
  return (
    <div className="flex flex-col">
      {[0, 1].map((index) => (
        <div
          key={index}
          className={cn(
            "flex items-center gap-3 px-4 py-3.5",
            index === 1 ? "" : "border-b border-border"
          )}
        >
          <div className="bg-default h-9 w-9 shrink-0 animate-pulse rounded-full" />
          <div className="flex flex-1 flex-col gap-1.5">
            <div className="bg-default h-3.5 w-32 animate-pulse rounded" />
            <div className="bg-default h-3 w-48 animate-pulse rounded" />
          </div>
        </div>
      ))}
    </div>
  )
}
