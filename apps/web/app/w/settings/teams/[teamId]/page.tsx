"use client"

import { use, useMemo, useState } from "react"
import NextLink from "next/link"
import { useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Chip, Skeleton, Spinner, Tooltip, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { queryKeys } from "@/lib/api/query-keys"
import { useAuth } from "@/lib/auth/auth-context"
import { useIsAdmin } from "@/lib/auth/use-role"
import {
  InviteMemberModal,
  TEAMS_KEY,
  TeamFormModal,
  formatDate,
  teamLabel,
  type Team,
} from "../_components/team-settings"
import {
  TeamDetailTabs,
  teamDetailTabs,
  type TeamDetailTab,
} from "./_team-detail-tabs"
import { TeamMembersSection } from "./team-sections"
import {
  TeamConnectionsSection,
  TeamKnowledgeSourcesSection,
  TeamSkillsSection,
} from "./team-provisioning"
import { TeamEnvironmentVariablesPanel } from "./team-environment-variables"
import { TeamExternalResourceRoutes } from "./team-external-resource-routes"

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
  const [archiveOpen, setArchiveOpen] = useState(false)
  const [activeTab, setActiveTab] = useState<TeamDetailTab>("overview")

  const teamQuery = $api.useQuery("get", "/v1/orgs/current/teams/{id}", {
    params: { path: { id: teamId } },
  })
  const teamsQuery = $api.useQuery("get", "/v1/orgs/current/teams", {
    params: { query: { limit: 100 } },
  })
  const membersQuery = $api.useQuery("get", "/v1/orgs/current/members", {})
  const archiveTeam = $api.useMutation("delete", "/v1/orgs/current/teams/{id}")

  const teams = useMemo(
    () => teamsQuery.data?.data ?? [],
    [teamsQuery.data?.data]
  )
  const team = teamQuery.data?.team as Team | undefined
  const members = useMemo(
    () => teamQuery.data?.members ?? [],
    [teamQuery.data?.members]
  )
  const orgMembers = useMemo(
    () => membersQuery.data?.data ?? [],
    [membersQuery.data?.data]
  )
  const tabs = teamDetailTabs(isAdmin)
  const visibleActiveTab = tabs.some((tab) => tab.id === activeTab)
    ? activeTab
    : "overview"

  function refreshTeam() {
    queryClient.invalidateQueries({
      queryKey: queryKeys.team(),
    })
    queryClient.invalidateQueries({ queryKey: TEAMS_KEY })
  }

  function handleArchive() {
    if (!team?.id || archiveTeam.isPending) return
    archiveTeam.mutate(
      { params: { path: { id: team.id } } },
      {
        onSuccess: () => {
          toast.success("Team archived")
          setArchiveOpen(false)
          queryClient.invalidateQueries({ queryKey: TEAMS_KEY })
          router.push("/w/settings/teams")
        },
        onError: (err) => {
          toast.danger(extractErrorMessage(err, "Could not archive team"))
          setArchiveOpen(false)
        },
      }
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
        <section className="overflow-hidden rounded-2xl border border-border bg-surface">
          <Skeleton className="h-16" />
        </section>
      </div>
    )
  }

  if (!team) {
    return (
      <div className="flex flex-col gap-6">
        <BackLink />
        <section className="rounded-2xl border border-border bg-surface px-4 py-8 text-center">
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
              "Manage this team's members and shared access."}
          </p>
          <div className="mt-3 flex flex-wrap gap-2">
            <Chip size="sm">{team.member_count ?? members.length} members</Chip>
            <Chip size="sm">Created {formatDate(team.created_at)}</Chip>
          </div>
        </div>
        {isAdmin ? (
          <div className="flex shrink-0 items-center gap-2">
            <Button
              variant="tertiary"
              size="sm"
              onPress={() => setEditOpen(true)}
            >
              <AppIcon icon="pencil" className="h-4 w-4" />
              Edit
            </Button>
            <ArchiveTeamButton
              pending={archiveTeam.isPending}
              isLastTeam={!teamsQuery.isLoading && teams.length <= 1}
              onPress={() => setArchiveOpen(true)}
            />
          </div>
        ) : null}
      </div>

      <TeamDetailTabs
        tabs={tabs}
        activeTab={visibleActiveTab}
        onSelect={setActiveTab}
      />

      {visibleActiveTab === "overview" ? (
        <div
          aria-labelledby="team-overview-tab"
          id="team-overview-panel"
          role="tabpanel"
        >
          <TeamMembersSection
            teamId={teamId}
            members={members}
            orgMembers={orgMembers}
            isLoading={membersQuery.isLoading}
            onInvite={() => setInviteOpen(true)}
            onChanged={refreshTeam}
            readOnly={!isAdmin}
          />
        </div>
      ) : visibleActiveTab === "connections" ? (
        <div
          aria-labelledby="team-connections-tab"
          className="flex flex-col gap-8"
          id="team-connections-panel"
          role="tabpanel"
        >
          <TeamConnectionsSection teamId={teamId} readOnly={!isAdmin} />
          <TeamExternalResourceRoutes teamId={teamId} />
        </div>
      ) : visibleActiveTab === "skills" ? (
        <div
          aria-labelledby="team-skills-tab"
          id="team-skills-panel"
          role="tabpanel"
        >
          <TeamSkillsSection teamId={teamId} readOnly={!isAdmin} />
        </div>
      ) : visibleActiveTab === "knowledge" && isAdmin ? (
        <div
          aria-labelledby="team-knowledge-tab"
          id="team-knowledge-panel"
          role="tabpanel"
        >
          <TeamKnowledgeSourcesSection teamId={teamId} />
        </div>
      ) : (
        <div
          aria-labelledby="team-environment-variables-tab"
          id="team-environment-variables-panel"
          role="tabpanel"
        >
          <TeamEnvironmentVariablesPanel teamId={teamId} />
        </div>
      )}

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

      <ConfirmDialog
        open={archiveOpen}
        pending={archiveTeam.isPending}
        heading={`Archive ${teamLabel(team)}?`}
        description="Archiving a team removes access to its shared resources. This cannot be undone."
        confirmLabel="Archive team"
        icon="archive"
        onOpenChange={setArchiveOpen}
        onConfirm={handleArchive}
      />
    </div>
  )
}

function ArchiveTeamButton({
  pending,
  isLastTeam,
  onPress,
}: {
  pending: boolean
  isLastTeam: boolean
  onPress: () => void
}) {
  const button = (
    <Button
      variant="danger-soft"
      size="sm"
      isDisabled={pending || isLastTeam}
      onPress={onPress}
    >
      {pending ? (
        <Spinner color="current" size="sm" />
      ) : (
        <AppIcon icon="archive" className="h-4 w-4" />
      )}
      Archive
    </Button>
  )

  if (!isLastTeam) return button

  return (
    <Tooltip delay={200} closeDelay={0}>
      <Tooltip.Trigger className="flex shrink-0">
        <span>{button}</span>
      </Tooltip.Trigger>
      <Tooltip.Content placement="top" offset={6} className="text-xs">
        An organization must keep at least one team.
      </Tooltip.Content>
    </Tooltip>
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
