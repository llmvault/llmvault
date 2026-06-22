"use client"

import { use, useMemo, useState } from "react"
import NextLink from "next/link"
import { useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Chip, ListBox, Select, Spinner, toast } from "@heroui/react"
import { Icon } from "@iconify/react"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { useAuth } from "@/lib/auth/auth-context"
import { cn } from "@/lib/utils"
import {
  CHANNELS_KEY,
  EmptyRow,
  InviteMemberModal,
  MEMBERS_KEY,
  RowSkeleton,
  TEAMS_KEY,
  TeamFormModal,
  channelLabel,
  formatDate,
  initials,
  memberLabel,
  roleLabel,
  teamLabel,
  type Channel,
  type Member,
  type Team,
  type TeamMember,
} from "../_components/team-settings"

export default function TeamDetailPage({
  params,
}: {
  params: Promise<{ teamId: string }>
}) {
  const { teamId } = use(params)
  const router = useRouter()
  const queryClient = useQueryClient()
  const { activeOrg } = useAuth()
  const isAdmin = activeOrg?.role === "owner" || activeOrg?.role === "admin"
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
              <Icon icon="lucide:users-round" className="h-4 w-4" />
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
          <div className="bg-default h-8 w-48 animate-pulse rounded" />
          <div className="bg-default h-4 w-72 animate-pulse rounded" />
        </div>
        <section className="bg-surface overflow-hidden rounded-2xl border border-border">
          <RowSkeleton />
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
            <Icon icon="lucide:pencil" className="h-4 w-4" />
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
              <Icon icon="lucide:archive" className="h-4 w-4" />
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
      <Icon icon="lucide:arrow-left" className="h-4 w-4" />
      Teams
    </NextLink>
  )
}

function TeamMembersSection({
  teamId,
  members,
  orgMembers,
  isLoading,
  onInvite,
  onChanged,
}: {
  teamId: string
  members: TeamMember[]
  orgMembers: Member[]
  isLoading: boolean
  onInvite: () => void
  onChanged: () => void
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
        <Button variant="tertiary" size="sm" onPress={onInvite}>
          <Icon icon="lucide:user-plus" className="h-4 w-4" />
          Invite to team
        </Button>
      </div>

      <div className="bg-surface rounded-2xl border border-border">
        <div className="flex flex-col gap-3 border-b border-border px-4 py-4 sm:flex-row sm:items-end">
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
              <Select.Trigger className="h-9 w-full justify-between rounded-md px-3 text-sm transition-colors">
                {selectedCandidate ? (
                  <span className="truncate">
                    {memberLabel(selectedCandidate)}
                  </span>
                ) : (
                  <Select.Value />
                )}
                <Select.Indicator />
              </Select.Trigger>
              <Select.Popover className="rounded-xl p-1.5">
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

        <div className="overflow-hidden">
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
              />
            ))
          )}
        </div>
      </div>
    </section>
  )
}

function TeamMemberRow({
  member,
  last,
  isBusy,
  onRemove,
}: {
  member: TeamMember
  last?: boolean
  isBusy: boolean
  onRemove: () => void
}) {
  return (
    <div
      className={cn(
        "flex items-center gap-3 px-4 py-3.5",
        last ? "" : "border-b border-border"
      )}
    >
      <div className="bg-default flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-xs font-medium text-muted">
        {initials(member.name, member.email)}
      </div>
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="truncate text-sm font-medium">
          {memberLabel(member)}
        </span>
        <span className="truncate text-sm text-muted">{member.email}</span>
      </div>
      <Chip size="sm">{roleLabel(member.role)}</Chip>
      <Button variant="ghost" size="sm" isDisabled={isBusy} onPress={onRemove}>
        Remove
      </Button>
    </div>
  )
}

function TeamChannelsSection({
  teamId,
  assignedChannels,
  availableChannels,
  isLoading,
  onChanged,
}: {
  teamId: string
  assignedChannels: Channel[]
  availableChannels: Channel[]
  isLoading: boolean
  onChanged: () => void
}) {
  const queryClient = useQueryClient()
  const updateChannel = $api.useMutation("patch", "/v1/channels/{id}")
  const [selectedChannelId, setSelectedChannelId] = useState("")
  const selectedChannel = availableChannels.find(
    (channel) => channel.id === selectedChannelId
  )

  function refreshChannels() {
    queryClient.invalidateQueries({ queryKey: CHANNELS_KEY })
    onChanged()
  }

  function handleAssign() {
    if (!selectedChannelId || updateChannel.isPending) return
    updateChannel.mutate(
      {
        params: { path: { id: selectedChannelId } },
        body: { team_id: teamId },
      },
      {
        onSuccess: () => {
          toast.success("Channel assigned")
          setSelectedChannelId("")
          refreshChannels()
        },
        onError: (err) =>
          toast.danger(extractErrorMessage(err, "Could not assign channel")),
      }
    )
  }

  function handleRemove(channel: Channel) {
    if (!channel.id || updateChannel.isPending) return
    updateChannel.mutate(
      {
        params: { path: { id: channel.id } },
        body: { team_id: "" },
      },
      {
        onSuccess: () => {
          toast.success("Channel removed from team")
          refreshChannels()
        },
        onError: (err) =>
          toast.danger(extractErrorMessage(err, "Could not update channel")),
      }
    )
  }

  return (
    <section className="flex flex-col gap-3">
      <div>
        <h2 className="text-sm font-medium">
          Channels
          {assignedChannels.length ? ` (${assignedChannels.length})` : ""}
        </h2>
        <p className="mt-1 text-sm text-muted">
          Channels without a team stay public to workspace members.
        </p>
      </div>

      <div className="bg-surface rounded-2xl border border-border">
        <div className="flex flex-col gap-3 border-b border-border px-4 py-4 sm:flex-row sm:items-end">
          <label className="flex min-w-0 flex-1 flex-col gap-1.5">
            <span className="text-sm font-medium">Assign public channel</span>
            <Select
              aria-label="Assign public channel"
              selectedKey={selectedChannelId || null}
              onSelectionChange={(key) =>
                setSelectedChannelId(key === null ? "" : String(key))
              }
              isDisabled={
                updateChannel.isPending || availableChannels.length === 0
              }
              className="w-full"
            >
              <Select.Trigger className="h-9 w-full justify-between rounded-md px-3 text-sm transition-colors">
                {selectedChannel ? (
                  <span className="truncate">
                    #{channelLabel(selectedChannel)}
                  </span>
                ) : (
                  <Select.Value />
                )}
                <Select.Indicator />
              </Select.Trigger>
              <Select.Popover className="rounded-xl p-1.5">
                <ListBox>
                  {availableChannels.map((channel) => (
                    <ListBox.Item
                      key={channel.id}
                      id={channel.id}
                      textValue={channelLabel(channel)}
                    >
                      #{channelLabel(channel)}
                    </ListBox.Item>
                  ))}
                </ListBox>
              </Select.Popover>
            </Select>
          </label>
          <Button
            variant="primary"
            size="sm"
            isDisabled={!selectedChannelId || updateChannel.isPending}
            onPress={handleAssign}
          >
            {updateChannel.isPending ? (
              <Spinner color="current" size="sm" />
            ) : null}
            Assign
          </Button>
        </div>

        <div className="overflow-hidden">
          {isLoading ? (
            <RowSkeleton />
          ) : assignedChannels.length === 0 ? (
            <EmptyRow text="No channels are assigned to this team." />
          ) : (
            assignedChannels.map((channel, index) => (
              <ChannelRow
                key={channel.id ?? index}
                channel={channel}
                last={index === assignedChannels.length - 1}
                isBusy={updateChannel.isPending}
                onRemove={() => handleRemove(channel)}
              />
            ))
          )}
        </div>
      </div>
    </section>
  )
}

function ChannelRow({
  channel,
  last,
  isBusy,
  onRemove,
}: {
  channel: Channel
  last?: boolean
  isBusy: boolean
  onRemove: () => void
}) {
  return (
    <div
      className={cn(
        "flex items-center gap-3 px-4 py-3.5",
        last ? "" : "border-b border-border"
      )}
    >
      <span className="bg-default flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-muted">
        <Icon icon="lucide:hash" className="h-4 w-4" />
      </span>
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="truncate text-sm font-medium">
          {channelLabel(channel)}
        </span>
        <span className="truncate text-sm text-muted">
          {channel.description || "No description"}
        </span>
      </div>
      <Button variant="ghost" size="sm" isDisabled={isBusy} onPress={onRemove}>
        Remove
      </Button>
    </div>
  )
}
