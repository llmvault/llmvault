"use client"

import { useMemo, useState } from "react"
import { Button } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { $api } from "@/lib/api/hooks"
import { useAuth } from "@/lib/auth/auth-context"
import { useIsAdmin } from "@/lib/auth/use-role"
import { ownerCount } from "./_components/member-actions"
import { TutorialBanner } from "@/components/tutorial-banner"
import { OrgMemberRow } from "./_components/member-lifecycle"
import {
  EmptyRow,
  InviteMemberModal,
  InviteRow,
  RowSkeleton,
  TeamFormModal,
  TeamRow,
  teamMap,
} from "./_components/team-settings"

export default function TeamsSettingsPage() {
  const { user, activeOrg } = useAuth()
  const isAdmin = useIsAdmin()
  const [inviteOpen, setInviteOpen] = useState(false)
  const [teamFormOpen, setTeamFormOpen] = useState(false)

  const teamsQuery = $api.useQuery(
    "get",
    "/v1/orgs/current/teams",
    { params: { query: { limit: 100 } } }
  )
  const membersQuery = $api.useQuery("get", "/v1/orgs/current/members")
  const invitesQuery = $api.useQuery(
    "get",
    "/v1/orgs/current/invites",
    {},
    { enabled: isAdmin }
  )

  const teams = useMemo(
    () => teamsQuery.data?.data ?? [],
    [teamsQuery.data?.data]
  )
  const members = useMemo(
    () => membersQuery.data?.data ?? [],
    [membersQuery.data?.data]
  )
  const invites = useMemo(
    () => invitesQuery.data?.data ?? [],
    [invitesQuery.data?.data]
  )
  const teamsById = useMemo(() => teamMap(teams), [teams])
  const memberOwnerCount = useMemo(() => ownerCount(members), [members])

  return (
    <div className="flex flex-col gap-10">
      <div className="flex items-start justify-between gap-4">
        <div className="flex flex-col gap-1">
          <h1 className="text-2xl font-semibold">Teams</h1>
          <p className="text-sm text-muted">
            Manage workspace members, invitations, and team-scoped resources.
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

      <TutorialBanner
        tutorial="teams"
        title="Understand team permissions"
        description="See how teams scope members, agents, connections, skills, and knowledge access."
        docsPath="workspace-and-access/teams"
      />

      <section className="flex flex-col gap-3">
        <div className="flex items-center justify-between gap-4">
          <h2 className="text-sm font-medium">
            Teams{teams.length ? ` (${teams.length})` : ""}
          </h2>
        </div>
        <div className="overflow-hidden rounded-2xl border border-border bg-surface">
          {teamsQuery.isLoading ? (
            <RowSkeleton />
          ) : teams.length === 0 ? (
            <EmptyRow
              text={
                isAdmin
                  ? "Create a team to group members and shared resources."
                  : "You're not a member of any teams yet."
              }
            />
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

      <section className="flex flex-col gap-3">
        <h2 className="text-sm font-medium">
          Members{members.length ? ` (${members.length})` : ""}
        </h2>
        <div className="overflow-hidden rounded-2xl border border-border bg-surface">
          {membersQuery.isLoading ? (
            <RowSkeleton />
          ) : members.length === 0 ? (
            <EmptyRow text="No members yet." />
          ) : (
            members.map((member, index) => (
              <OrgMemberRow
                key={member.user_id ?? member.email ?? index}
                member={member}
                isYou={member.user_id === user?.id}
                actorRole={activeOrg?.role}
                ownerCount={memberOwnerCount}
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
          <div className="overflow-hidden rounded-2xl border border-border bg-surface">
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
