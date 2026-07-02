"use client"

import { useMemo, useState } from "react"
import { Button } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { $api } from "@/lib/api/hooks"
import { useAuth } from "@/lib/auth/auth-context"
import {
  EmptyRow,
  InviteMemberModal,
  InviteRow,
  MemberRow,
  RowSkeleton,
  TeamFormModal,
  TeamRow,
  teamMap,
  type Invite,
  type Member,
  type Team,
} from "./_components/team-settings"

export default function TeamsSettingsPage() {
  const { user, activeOrg } = useAuth()
  const isAdmin = activeOrg?.role === "owner" || activeOrg?.role === "admin"
  const [inviteOpen, setInviteOpen] = useState(false)
  const [teamFormOpen, setTeamFormOpen] = useState(false)

  const teamsQuery = $api.useQuery(
    "get",
    "/v1/orgs/current/teams",
    { params: { query: { limit: 100 } } },
    { enabled: isAdmin }
  )
  const membersQuery = $api.useQuery("get", "/v1/orgs/current/members")
  const invitesQuery = $api.useQuery(
    "get",
    "/v1/orgs/current/invites",
    {},
    { enabled: isAdmin }
  )

  const teams = useMemo(
    () => (teamsQuery.data?.data ?? []) as Team[],
    [teamsQuery.data?.data]
  )
  const members = useMemo(
    () => (membersQuery.data?.data ?? []) as Member[],
    [membersQuery.data?.data]
  )
  const invites = useMemo(
    () => (invitesQuery.data?.data ?? []) as Invite[],
    [invitesQuery.data?.data]
  )
  const teamsById = useMemo(() => teamMap(teams), [teams])

  return (
    <div className="flex flex-col gap-10">
      <div className="flex items-start justify-between gap-4">
        <div className="flex flex-col gap-1">
          <h1 className="text-2xl font-semibold">Teams</h1>
          <p className="text-sm text-muted">
            Manage workspace members, invitations, and team-scoped channels.
          </p>
        </div>
        {isAdmin ? (
          <div className="flex shrink-0 items-center gap-2">
            <Button
              variant="tertiary"
              size="sm"
              onPress={() => setInviteOpen(true)}
            >
              <AppIcon icon="user-plus" className="h-4 w-4" />
              Invite member
            </Button>
            <Button
              variant="primary"
              size="sm"
              onPress={() => setTeamFormOpen(true)}
            >
              <AppIcon icon="plus" className="h-4 w-4" />
              New team
            </Button>
          </div>
        ) : null}
      </div>

      {isAdmin ? (
        <section className="flex flex-col gap-3">
          <div className="flex items-center justify-between gap-4">
            <h2 className="text-sm font-medium">
              Teams{teams.length ? ` (${teams.length})` : ""}
            </h2>
          </div>
          <div className="bg-surface overflow-hidden rounded-2xl border border-border">
            {teamsQuery.isLoading ? (
              <RowSkeleton />
            ) : teams.length === 0 ? (
              <EmptyRow text="Create a team to group members and private channels." />
            ) : (
              teams.map((team, index) => (
                <TeamRow
                  key={team.id ?? index}
                  team={team}
                  last={index === teams.length - 1}
                />
              ))
            )}
          </div>
        </section>
      ) : (
        <section className="bg-surface rounded-2xl border border-border px-4 py-4">
          <div className="flex items-start gap-3">
            <div className="bg-default flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-muted">
              <AppIcon icon="users-round" className="h-4 w-4" />
            </div>
            <div className="min-w-0">
              <h2 className="text-sm font-medium">Team management</h2>
              <p className="mt-1 text-sm leading-5 text-muted">
                Workspace admins manage teams and private channel access.
              </p>
            </div>
          </div>
        </section>
      )}

      <section className="flex flex-col gap-3">
        <h2 className="text-sm font-medium">
          Members{members.length ? ` (${members.length})` : ""}
        </h2>
        <div className="bg-surface overflow-hidden rounded-2xl border border-border">
          {membersQuery.isLoading ? (
            <RowSkeleton />
          ) : members.length === 0 ? (
            <EmptyRow text="No members yet." />
          ) : (
            members.map((member, index) => (
              <MemberRow
                key={member.user_id ?? member.email ?? index}
                member={member}
                isYou={member.user_id === user?.id}
                last={index === members.length - 1}
              />
            ))
          )}
        </div>
      </section>

      {isAdmin ? (
        <section className="flex flex-col gap-3">
          <h2 className="text-sm font-medium">
            Pending invitations{invites.length ? ` (${invites.length})` : ""}
          </h2>
          <div className="bg-surface overflow-hidden rounded-2xl border border-border">
            {invitesQuery.isLoading ? (
              <RowSkeleton />
            ) : invites.length === 0 ? (
              <EmptyRow text="No pending invitations." />
            ) : (
              invites.map((invite, index) => (
                <InviteRow
                  key={invite.id ?? index}
                  invite={invite}
                  teamsById={teamsById}
                  last={index === invites.length - 1}
                />
              ))
            )}
          </div>
        </section>
      ) : null}

      {inviteOpen ? (
        <InviteMemberModal
          open={inviteOpen}
          onOpenChange={setInviteOpen}
          orgName={activeOrg?.name}
          teams={teams}
        />
      ) : null}
      {teamFormOpen ? (
        <TeamFormModal open={teamFormOpen} onOpenChange={setTeamFormOpen} />
      ) : null}
    </div>
  )
}
