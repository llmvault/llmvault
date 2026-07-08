"use client"

import { useState } from "react"
import { Button, Skeleton, Spinner, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import {
  agentName,
  installErrorMessage,
  teamHasAgent,
  teamNameByID,
  type CatalogAgent,
  type Team,
} from "../_lib"

// Team-scoped install/uninstall for a catalog agent. Each of the caller's
// visible teams gets a row: install into teams that don't have a clone yet,
// uninstall the clone from teams that do. Installing/uninstalling is member
// reachable (the backend gates on team membership), so there is no admin gate
// here — the caller only ever sees their own teams.
export function AgentTeamsSection({
  slug,
  agent,
  teams,
  isLoading,
  onChanged,
}: {
  slug: string
  agent: CatalogAgent
  teams: Team[]
  isLoading: boolean
  onChanged: () => void
}) {
  // The team currently being installed/uninstalled, so we can show a spinner on
  // just that row while the shared mutations are in flight.
  const [pendingTeamID, setPendingTeamID] = useState("")
  const [uninstallTeam, setUninstallTeam] = useState<Team | null>(null)
  const installAgent = $api.useMutation("post", "/v1/agents/catalog/{slug}/install")
  const uninstallAgent = $api.useMutation(
    "delete",
    "/v1/agents/catalog/{slug}/install"
  )

  function handleInstall(team: Team) {
    const teamID = team.id
    if (!teamID) return
    setPendingTeamID(teamID)
    installAgent.mutate(
      { params: { path: { slug } }, body: { team_id: teamID } },
      {
        onSuccess: () => {
          toast.success(
            `${agentName(agent)} installed for ${teamNameByID(teams, teamID)}`
          )
          onChanged()
        },
        onError: (error) =>
          toast.danger(
            installErrorMessage(error, teamNameByID(teams, teamID), agent)
          ),
        onSettled: () => setPendingTeamID(""),
      }
    )
  }

  function handleUninstall() {
    const team = uninstallTeam
    const teamID = team?.id
    if (!teamID) return
    setPendingTeamID(teamID)
    uninstallAgent.mutate(
      { params: { path: { slug }, query: { team_id: teamID } } },
      {
        onSuccess: () => {
          toast.success(
            `${agentName(agent)} uninstalled from ${teamNameByID(teams, teamID)}`
          )
          setUninstallTeam(null)
          onChanged()
        },
        onError: (error) =>
          toast.danger(
            extractErrorMessage(error, "Could not uninstall agent")
          ),
        onSettled: () => setPendingTeamID(""),
      }
    )
  }

  return (
    <section className="flex flex-col gap-3">
      <div>
        <h2 className="text-sm font-semibold text-foreground">Teams</h2>
        <p className="text-muted-foreground mt-1 text-sm">
          Install {agentName(agent)} for the teams you belong to.
        </p>
      </div>

      {isLoading ? (
        <TeamsSkeleton />
      ) : teams.length === 0 ? (
        <NoTeamsState />
      ) : (
        <div className="bg-card flex flex-col divide-y divide-border overflow-hidden rounded-xl border border-border">
          {teams.map((team) => (
            <TeamRow
              key={team.id ?? team.name}
              team={team}
              installed={Boolean(team.id && teamHasAgent(agent, team.id))}
              busy={pendingTeamID === team.id}
              disabled={Boolean(pendingTeamID) && pendingTeamID !== team.id}
              onInstall={() => handleInstall(team)}
              onUninstall={() => setUninstallTeam(team)}
            />
          ))}
        </div>
      )}

      <ConfirmDialog
        open={uninstallTeam !== null}
        pending={uninstallAgent.isPending}
        heading="Uninstall agent"
        description={`This removes ${agentName(agent)} from ${
          uninstallTeam ? teamNameByID(teams, uninstallTeam.id ?? "") : "this team"
        } along with its settings. You can install it again later.`}
        confirmLabel="Uninstall"
        onOpenChange={(open) => {
          if (!open) setUninstallTeam(null)
        }}
        onConfirm={handleUninstall}
      />
    </section>
  )
}

function TeamRow({
  team,
  installed,
  busy,
  disabled,
  onInstall,
  onUninstall,
}: {
  team: Team
  installed: boolean
  busy: boolean
  disabled: boolean
  onInstall: () => void
  onUninstall: () => void
}) {
  return (
    <div className="flex items-center justify-between gap-3 px-3 py-2.5">
      <div className="flex min-w-0 flex-1 items-center gap-3 overflow-hidden">
        <div className="text-muted-foreground flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-default">
          <AppIcon icon="users" className="h-4 w-4" />
        </div>
        <div className="min-w-0 flex-1 overflow-hidden">
          <p className="block truncate text-sm font-medium text-foreground">
            {team.name || "Untitled team"}
          </p>
          <p className="text-muted-foreground text-xs">
            {installed ? "Installed" : "Not installed"}
          </p>
        </div>
      </div>

      {installed ? (
        <Button
          size="sm"
          variant="secondary"
          className="shrink-0 text-danger"
          isDisabled={busy || disabled}
          onPress={onUninstall}
        >
          {busy ? (
            <Spinner color="current" size="sm" />
          ) : (
            <AppIcon icon="trash-2" className="h-4 w-4" />
          )}
          Uninstall
        </Button>
      ) : (
        <Button
          size="sm"
          variant="primary"
          className="shrink-0"
          isDisabled={busy || disabled}
          onPress={onInstall}
        >
          {busy ? <Spinner color="current" size="sm" /> : null}
          Install
        </Button>
      )}
    </div>
  )
}

function TeamsSkeleton() {
  return (
    <div className="bg-card flex flex-col divide-y divide-border overflow-hidden rounded-xl border border-border">
      {Array.from({ length: 2 }).map((_, index) => (
        <div
          key={index}
          className="flex items-center justify-between gap-3 px-3 py-2.5"
        >
          <div className="flex min-w-0 flex-1 items-center gap-3">
            <Skeleton className="h-8 w-8" />
            <Skeleton className="h-4 w-32 rounded" />
          </div>
          <Skeleton className="h-8 w-20 rounded-md" />
        </div>
      ))}
    </div>
  )
}

function NoTeamsState() {
  return (
    <div className="bg-card flex items-center gap-3 rounded-xl border border-border px-3 py-2.5">
      <div className="text-muted-foreground flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-default">
        <AppIcon icon="users" className="h-4 w-4" />
      </div>
      <div className="min-w-0">
        <p className="text-sm font-medium text-foreground">No teams available</p>
        <p className="text-muted-foreground text-xs">
          Join or create a team to install this agent.
        </p>
      </div>
    </div>
  )
}
