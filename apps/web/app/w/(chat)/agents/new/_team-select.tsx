"use client"

import { ListBox, Select } from "@heroui/react"
import { $api } from "@/lib/api/hooks"

const NO_TEAM_KEY = ""

// Team picker for the agent form. Backed by GET /v1/orgs/current/teams, which
// is member-reachable and scopes its result to the caller: org managers see
// every team, a plain member sees only the teams they belong to. The field is
// optional (empty = "no team", a manager-only choice enforced server-side).
export function TeamSelect({
  value,
  onValueChange,
  disabled,
}: {
  value: string
  onValueChange: (teamId: string) => void
  disabled?: boolean
}) {
  const teamsQuery = $api.useQuery("get", "/v1/orgs/current/teams", {
    params: { query: { limit: 100 } },
  })
  const teams = teamsQuery.data?.data ?? []
  const selectedTeam = teams.find((team) => team.id === value)
  const selectedLabel = teamsQuery.isLoading
    ? "Loading teams"
    : selectedTeam?.name || "No team"

  return (
    <Select
      aria-label="Team"
      selectedKey={value || NO_TEAM_KEY}
      onSelectionChange={(key) => {
        if (key !== null) onValueChange(String(key))
      }}
      isDisabled={disabled || teamsQuery.isLoading}
      className="w-full"
    >
      <Select.Trigger className="h-9 w-full justify-between px-3 text-sm transition-colors">
        <span className="truncate">{selectedLabel}</span>
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover className="p-1.5">
        <ListBox>
          <ListBox.Item id={NO_TEAM_KEY} textValue="No team">
            <span className="text-sm font-medium">No team</span>
          </ListBox.Item>
          {teams.map((team) => (
            <ListBox.Item
              key={team.id}
              id={team.id ?? ""}
              textValue={team.name || team.id || "Team"}
            >
              <span className="flex min-w-0 flex-col gap-0.5">
                <span className="truncate text-sm font-medium">
                  {team.name || "Untitled team"}
                </span>
                {team.description ? (
                  <span className="text-muted-foreground truncate text-xs">
                    {team.description}
                  </span>
                ) : null}
              </span>
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}
