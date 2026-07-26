"use client"

import { useState } from "react"
import { useQueries } from "@tanstack/react-query"
import { Input, Skeleton } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { $api } from "@/lib/api/hooks"
import { cn } from "@/lib/utils"
import type { components } from "@/lib/api/schema"
import { useWorkspace } from "@/app/w/(chat)/_components/shell"
import { CollectionState } from "@/app/w/(chat)/_components/collection-state"
import { usePanelSheetTargetStore } from "@/app/w/(chat)/_stores/panel-sheet-target-store"

type Team = components["schemas"]["teamResponse"]
type SheetSummary = components["schemas"]["sheetSummary"]

export default function SheetsPage() {
  const [query, setQuery] = useState("")
  const { openView, startNewChat } = useWorkspace()
  const openSheet = usePanelSheetTargetStore((state) => state.openSheet)
  const activeSheetId = usePanelSheetTargetStore(
    (state) => state.target?.sheetId ?? null
  )

  const teamsQuery = $api.useQuery("get", "/v1/orgs/current/teams", {
    params: { query: { limit: 100 } },
  })
  const teams = (teamsQuery.data?.data ?? []).filter(
    (team): team is Team & { id: string } => Boolean(team.id)
  )
  const sheetQueries = useQueries({
    queries: teams.map((team) =>
      $api.queryOptions("get", "/v1/sheets", {
        params: { query: { team_id: team.id, limit: 200 } },
      })
    ),
  })
  const normalized = query.trim().toLowerCase()
  const groups = teams.map((team, index) => {
    const allSheets = sheetQueries[index]?.data?.sheets ?? []
    const sheets = allSheets.filter(
      (sheet) =>
        !normalized ||
        (sheet.name ?? "").toLowerCase().includes(normalized) ||
        (sheet.description ?? "").toLowerCase().includes(normalized)
    )
    return { team, sheets, total: allSheets.length }
  })
  const totalSheets = groups.reduce((total, group) => total + group.total, 0)
  const matchingSheets = groups.reduce(
    (total, group) => total + group.sheets.length,
    0
  )
  const isLoading =
    teamsQuery.isPending || sheetQueries.some((result) => result.isPending)
  const isError =
    teamsQuery.isError || sheetQueries.some((result) => result.isError)

  // Open the clicked sheet in the shared right panel (the same one that
  // slides open for a session), not a bespoke panel.
  const handleOpen = (teamId: string, sheetId: string) => {
    openSheet(teamId, sheetId)
    openView("sheets")
  }

  return (
    <div className="h-full overflow-y-auto bg-background text-foreground">
      <div className="mx-auto w-full max-w-2xl px-6 py-12">
        <div className="flex flex-col gap-8">
          <nav aria-label="Sheets" className="flex items-center gap-1">
            <span className="rounded-lg bg-default px-3 py-1.5 text-sm font-medium text-foreground">
              Sheets
            </span>
          </nav>

          <div>
            <h1 className="text-lg font-semibold text-foreground">Sheets</h1>
            <p className="text-muted-foreground mt-1 text-sm">
              Structured data your agents keep, organised by team
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
              placeholder="Search sheets"
              className="bg-card h-10 w-full rounded-md pl-9"
            />
          </div>

          {isLoading ? (
            <ListSkeleton />
          ) : isError ? (
            <CollectionState
              icon="triangle-alert"
              title="Could not load sheets"
              description="Try again to load sheets across your teams."
              action={{
                label: "Try again",
                icon: "refresh-cw",
                variant: "secondary",
                onPress: () => {
                  void teamsQuery.refetch()
                  sheetQueries.forEach((result) => void result.refetch())
                },
              }}
            />
          ) : teams.length === 0 ? (
            <CollectionState
              icon="users"
              title="No teams yet"
              description="Sheets are organised by team. Create or join a team to get started."
            />
          ) : totalSheets === 0 ? (
            <CollectionState
              icon="table"
              title="No sheets yet"
              description="Ask an agent to collect or organise data in a sheet. It will appear here when it is ready."
              action={{
                label: "Start a chat",
                icon: "square-pen",
                variant: "primary",
                onPress: startNewChat,
              }}
            />
          ) : matchingSheets === 0 ? (
            <CollectionState
              icon="search"
              title="No sheets found"
              description="Try a different search term or clear the current search."
              action={{
                label: "Clear search",
                icon: "x",
                variant: "secondary",
                onPress: () => setQuery(""),
              }}
            />
          ) : (
            <div className="flex flex-col gap-8">
              {groups.map(({ team, sheets }) => (
                <TeamSection
                  key={team.id}
                  team={team}
                  sheets={sheets}
                  activeSheetId={activeSheetId}
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
  sheets,
  activeSheetId,
  onOpen,
}: {
  team: Team & { id: string }
  sheets: SheetSummary[]
  activeSheetId: string | null
  onOpen: (teamId: string, sheetId: string) => void
}) {
  if (sheets.length === 0) return null

  return (
    <section className="flex flex-col gap-3">
      <div className="flex items-center gap-2">
        <AppIcon
          icon="users"
          className="text-muted-foreground h-3.5 w-3.5"
          aria-hidden="true"
        />
        <h2 className="text-sm font-medium text-foreground">{team.name}</h2>
        <span className="text-muted-foreground text-xs">{sheets.length}</span>
      </div>
      <div className="bg-card flex flex-col">
        {sheets.map((sheet) => (
          <SheetRow
            key={sheet.id}
            sheet={sheet}
            active={activeSheetId === sheet.id}
            onOpen={() => onOpen(team.id, sheet.id ?? "")}
          />
        ))}
      </div>
    </section>
  )
}

function SheetRow({
  sheet,
  active,
  onOpen,
}: {
  sheet: SheetSummary
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
          <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-emerald-600 text-white">
            <AppIcon icon="table" className="h-[18px] w-[18px]" />
          </div>

          <div className="min-w-0 flex-1">
            <h3 className="text-sm font-medium text-foreground">
              {sheet.name}
            </h3>
            {sheet.description ? (
              <p className="text-muted-foreground truncate text-sm">
                {sheet.description}
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
