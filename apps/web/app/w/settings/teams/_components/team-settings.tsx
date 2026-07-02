"use client"

import { useMemo } from "react"
import NextLink from "next/link"
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
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { cn } from "@/lib/utils"
import type { components } from "@/lib/api/schema"

export { InviteMemberModal, TeamFormModal } from "./team-settings-modals"

export type Team = components["schemas"]["teamResponse"]
export type Member = components["schemas"]["orgMemberResponse"]
export type Invite = components["schemas"]["orgInviteResponse"]
export type TeamMember = components["schemas"]["teamMemberResponse"]
export type Channel = components["schemas"]["channelResponse"]

export const TEAMS_KEY = ["get", "/v1/orgs/current/teams"] as const
export const MEMBERS_KEY = ["get", "/v1/orgs/current/members"] as const
export const INVITES_KEY = ["get", "/v1/orgs/current/invites"] as const
export const CHANNELS_KEY = ["get", "/v1/channels"] as const

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
        <AppIcon icon="users-round" className="h-4 w-4" />
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
      <AppIcon
        icon="chevron-right"
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
          <AppIcon icon="mail" className="h-4 w-4" />
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
