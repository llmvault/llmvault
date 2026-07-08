"use client"

import { use, useMemo, useState } from "react"
import NextLink from "next/link"
import { useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Chip, Skeleton, Spinner, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { useAuth } from "@/lib/auth/auth-context"
import { useIsAdmin } from "@/lib/auth/use-role"
import {
  InviteMemberModal,
  TEAMS_KEY,
  TeamFormModal,
  formatDate,
  teamLabel,
  type Channel,
  type Member,
  type Team,
  type TeamMember,
} from "../_components/team-settings"
import { TeamChannelsSection, TeamMembersSection } from "./team-sections"
import { TeamProvisioningSection } from "./team-provisioning"

export default function TeamDetailPage({
  params,
}: {
  params: Promise<{ teamId: string }>
}) {
  const { teamId } = use(params)
  const router = useRouter()
  const queryClient = useQueryClient()
  const { activeOrg } = useAuth()
  const isAdmin = useIsAdmin()
  const [inviteOpen, setInviteOpen] = useState(false)
  const [editOpen, setEditOpen] = useState(false)

  const teamQuery = $api.useQuery(
    "get",
    "/v1/orgs/current/teams/{id}",
    { params: { path: { id: teamId } } },
    { enabled: isAdmin }
  )
  const teamsQuery = $api.useQuery(
    "get",
    "/v1/orgs/current/teams",
    { params: { query: { limit: 100 } } },
    { enabled: isAdmin }
  )
  const membersQuery = $api.useQuery(
    "get",
    "/v1/orgs/current/members",
    {},
    { enabled: isAdmin }
  )
  const channelsQuery = $api.useQuery(
    "get",
    "/v1/channels",
    { params: { query: { limit: 100 } } },
    { enabled: isAdmin }
  )
  const archiveTeam = $api.useMutation("delete", "/v1/orgs/current/teams/{id}")

  const teams = useMemo(
    () => (teamsQuery.data?.data ?? []) as Team[],
    [teamsQuery.data?.data]
  )
  const team = teamQuery.data?.team as Team | undefined
  const members = useMemo(
    () => (teamQuery.data?.members ?? []) as TeamMember[],
    [teamQuery.data?.members]
  )
  const orgMembers = useMemo(
    () => (membersQuery.data?.data ?? []) as Member[],
    [membersQuery.data?.data]
  )
  const channels = useMemo(
    () => (channelsQuery.data?.data ?? []) as Channel[],
    [channelsQuery.data?.data]
  )
  const assignedChannels = useMemo(
    () => channels.filter((channel) => channel.team_id === teamId),
    [channels, teamId]
  )
  const availableChannels = useMemo(
    () => channels.filter((channel) => !channel.team_id),
    [channels]
  )

  function refreshTeam() {
    queryClient.invalidateQueries({
      queryKey: ["get", "/v1/orgs/current/teams/{id}"],
    })
    queryClient.invalidateQueries({ queryKey: TEAMS_KEY })
  }

  function handleArchive() {
    if (!team?.id || archiveTeam.isPending) return
    const confirmed = window.confirm(
      `Archive ${teamLabel(team)}? Remove channels from this team first.`
    )
    if (!confirmed) return
    archiveTeam.mutate(
      { params: { path: { id: team.id } } },
      {
        onSuccess: () => {
          toast.success("Team archived")
          queryClient.invalidateQueries({ queryKey: TEAMS_KEY })
          router.push("/w/settings/teams")
        },
        onError: (err) =>
          toast.danger(extractErrorMessage(err, "Could not archive team")),
      }
    )
  }

  if (!isAdmin) {
    return (
      <div className="flex flex-col gap-6">
        <BackLink />
        <section className="bg-surface rounded-2xl border border-border px-4 py-4">
          <div className="flex items-start gap-3">
            <div className="bg-default flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-muted">
              <AppIcon icon="users-round" className="h-4 w-4" />
            </div>
            <div className="min-w-0">
              <h1 className="text-sm font-medium">Team management</h1>
              <p className="mt-1 text-sm leading-5 text-muted">
                Workspace admins manage teams and private channel access.
              </p>
            </div>
          </div>
        </section>
      </div>
    )
  }

  if (teamQuery.isLoading) {
    return (
      <div className="flex flex-col gap-8">
        <BackLink />
        <div className="flex flex-col gap-2">
          <Skeleton className="h-8 w-48 rounded" />
          <Skeleton className="h-4 w-72 rounded" />
        </div>
        <section className="bg-surface overflow-hidden rounded-2xl border border-border">
          <Skeleton className="h-16" />
        </section>
      </div>
    )
  }

  if (!team) {
    return (
      <div className="flex flex-col gap-6">
        <BackLink />
        <section className="bg-surface rounded-2xl border border-border px-4 py-8 text-center">
          <h1 className="text-sm font-medium">Team not found</h1>
          <p className="mt-1 text-sm text-muted">
            This team may have been archived.
          </p>
        </section>
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-8">
      <BackLink />

      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <h1 className="truncate text-2xl font-semibold">{teamLabel(team)}</h1>
          <p className="mt-1 text-sm text-muted">
            {team.description ||
              "Manage this team's members and channel access."}
          </p>
          <div className="mt-3 flex flex-wrap gap-2">
            <Chip size="sm">{team.member_count ?? members.length} members</Chip>
            <Chip size="sm">
              {team.channel_count ?? assignedChannels.length} channels
            </Chip>
            <Chip size="sm">Created {formatDate(team.created_at)}</Chip>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Button
            variant="tertiary"
            size="sm"
            onPress={() => setEditOpen(true)}
          >
            <AppIcon icon="pencil" className="h-4 w-4" />
            Edit
          </Button>
          <Button
            variant="danger-soft"
            size="sm"
            isDisabled={archiveTeam.isPending}
            onPress={handleArchive}
          >
            {archiveTeam.isPending ? (
              <Spinner color="current" size="sm" />
            ) : (
              <AppIcon icon="archive" className="h-4 w-4" />
            )}
            Archive
          </Button>
        </div>
      </div>

      <TeamMembersSection
        teamId={teamId}
        members={members}
        orgMembers={orgMembers}
        isLoading={membersQuery.isLoading}
        onInvite={() => setInviteOpen(true)}
        onChanged={refreshTeam}
      />

      <TeamChannelsSection
        teamId={teamId}
        assignedChannels={assignedChannels}
        availableChannels={availableChannels}
        isLoading={channelsQuery.isLoading}
        onChanged={refreshTeam}
      />

      <TeamProvisioningSection teamId={teamId} />

      {inviteOpen ? (
        <InviteMemberModal
          open={inviteOpen}
          onOpenChange={setInviteOpen}
          orgName={activeOrg?.name}
          teams={teams.length > 0 ? teams : [team]}
          defaultTeamIds={[teamId]}
        />
      ) : null}
      {editOpen ? (
        <TeamFormModal
          open={editOpen}
          onOpenChange={setEditOpen}
          initialTeam={team}
        />
      ) : null}
    </div>
  )
}

function BackLink() {
  return (
    <NextLink
      href="/w/settings/teams"
      className="inline-flex w-fit items-center gap-2 text-sm text-muted transition-colors hover:text-foreground"
    >
      <AppIcon icon="arrow-left" className="h-4 w-4" />
      Teams
    </NextLink>
  )
}
