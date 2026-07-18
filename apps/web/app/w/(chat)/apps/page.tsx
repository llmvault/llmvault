"use client"

import { useState } from "react"
import { Input, Skeleton } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { $api } from "@/lib/api/hooks"
import { cn } from "@/lib/utils"
import type { components } from "@/lib/api/schema"
import { useWorkspace } from "@/app/w/(chat)/_components/shell"
import { usePanelAppTargetStore } from "@/app/w/(chat)/_stores/panel-app-target-store"

type Team = components["schemas"]["teamResponse"]
type AppView = components["schemas"]["appView"]

export default function AppsPage() {
  const [query, setQuery] = useState("")
  const { openView } = useWorkspace()
  const openApp = usePanelAppTargetStore((state) => state.openApp)
  const activeAppId = usePanelAppTargetStore(
    (state) => state.target?.appId ?? null
  )

  const teamsQuery = $api.useQuery("get", "/v1/orgs/current/teams", {
    params: { query: { limit: 100 } },
  })
  const teams = teamsQuery.data?.data ?? []

  // Open the clicked app in the shared right panel (the same one that slides
  // open for a session), not a bespoke panel.
  const handleOpen = (app: AppView) => {
    openApp(app.id ?? "", app.name ?? "Untitled app")
    openView("apps")
  }

  return (
    <div className="h-full overflow-y-auto bg-background text-foreground">
      <div className="mx-auto w-full max-w-2xl px-6 py-12">
        <div className="flex flex-col gap-8">
          <nav aria-label="Apps" className="flex items-center gap-1">
            <span className="rounded-lg bg-default px-3 py-1.5 text-sm font-medium text-foreground">
              Apps
            </span>
          </nav>

          <div>
            <h1 className="text-lg font-semibold text-foreground">Apps</h1>
            <p className="text-muted-foreground mt-1 text-sm">
              Custom apps your agents build, organised by team
            </p>
          </div>

          <div className="relative min-w-0">
            <AppIcon
              icon="search"
              className="text-muted-foreground pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2"
            />
            <Input
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Search apps"
              className="bg-card h-10 w-full rounded-md pl-9"
            />
          </div>

          {teamsQuery.isPending ? (
            <ListSkeleton />
          ) : teams.length === 0 ? (
            <EmptyState message="No teams yet." />
          ) : (
            <div className="flex flex-col gap-8">
              {teams.map((team) => (
                <TeamSection
                  key={team.id}
                  team={team}
                  query={query}
                  activeAppId={activeAppId}
                  onOpen={handleOpen}
                />
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

function TeamSection({
  team,
  query,
  activeAppId,
  onOpen,
}: {
  team: Team
  query: string
  activeAppId: string | null
  onOpen: (app: AppView) => void
}) {
  const teamId = team.id ?? ""
  const appsQuery = $api.useQuery(
    "get",
    "/v1/apps",
    { params: { query: { team_id: teamId } } },
    { enabled: Boolean(teamId) }
  )

  const normalized = query.trim().toLowerCase()
  const apps = (appsQuery.data?.apps ?? []).filter(
    (app) =>
      !normalized ||
      (app.name ?? "").toLowerCase().includes(normalized) ||
      (app.description ?? "").toLowerCase().includes(normalized)
  )

  // Keep the list quiet: teams with no (matching) apps are hidden.
  if (apps.length === 0) return null

  return (
    <section className="flex flex-col gap-3">
      <div className="flex items-center gap-2">
        <AppIcon
          icon="users"
          className="text-muted-foreground h-3.5 w-3.5"
          aria-hidden="true"
        />
        <h2 className="text-sm font-medium text-foreground">{team.name}</h2>
        <span className="text-muted-foreground text-xs">{apps.length}</span>
      </div>
      <div className="bg-card flex flex-col">
        {apps.map((app) => (
          <AppRow
            key={app.id}
            app={app}
            active={activeAppId === app.id}
            onOpen={() => onOpen(app)}
          />
        ))}
      </div>
    </section>
  )
}

function AppRow({
  app,
  active,
  onOpen,
}: {
  app: AppView
  active: boolean
  onOpen: () => void
}) {
  return (
    <button
      type="button"
      onClick={onOpen}
      className="group -mx-3 block w-full py-1.5 text-left"
    >
      <div
        className={cn(
          "rounded-xl px-3 py-1.5 transition-colors group-focus-visible:outline-2 group-focus-visible:outline-offset-2 group-focus-visible:outline-foreground/40",
          active ? "bg-default" : "group-hover:bg-default"
        )}
      >
        <div className="flex items-center gap-3">
          <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-violet-600 text-white">
            <AppIcon
              icon={app.icon || "layout-grid"}
              className="h-[18px] w-[18px]"
            />
          </div>

          <div className="min-w-0 flex-1">
            <h3 className="text-sm font-medium text-foreground">{app.name}</h3>
            {app.description ? (
              <p className="text-muted-foreground truncate text-sm">
                {app.description}
              </p>
            ) : null}
          </div>

          <AppIcon
            icon="chevron-right"
            className="text-muted-foreground h-4 w-4 shrink-0 transition-colors group-hover:text-foreground"
            aria-hidden="true"
          />
        </div>
      </div>
    </button>
  )
}

function ListSkeleton() {
  return (
    <div className="flex flex-col gap-8">
      {[0, 1].map((section) => (
        <div key={section} className="flex flex-col gap-3">
          <Skeleton className="h-3.5 w-24 rounded" />
          <div className="bg-card flex flex-col">
            {[0, 1, 2].map((row) => (
              <div key={row} className="flex items-center gap-3 px-3 py-2.5">
                <Skeleton className="h-9 w-9 shrink-0" />
                <div className="flex min-w-0 flex-1 flex-col gap-2">
                  <Skeleton className="h-3.5 w-32 rounded" />
                  <Skeleton className="h-3 w-56 max-w-full rounded" />
                </div>
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}

function EmptyState({ message }: { message: string }) {
  return (
    <div className="bg-card flex min-h-56 flex-col items-center justify-center rounded-xl px-6 text-center">
      <AppIcon icon="layout-grid" className="text-muted-foreground h-7 w-7" />
      <p className="mt-3 text-sm font-medium text-foreground">{message}</p>
      <p className="text-muted-foreground mt-1 max-w-sm text-sm">
        Ask an agent to build an app and it will show up here.
      </p>
    </div>
  )
}
