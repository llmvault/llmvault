"use client"

import { useMemo } from "react"
import { ListBox, Select } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"

type Team = components["schemas"]["teamResponse"]

export function useTeams() {
  const query = $api.useQuery("get", "/v1/orgs/current/teams", {
    params: { query: { limit: 100 } },
  })
  const teams = useMemo(
    () => (query.data?.data ?? []).filter((team) => Boolean(team.id)),
    [query.data?.data]
  )
  return { teams, isLoading: query.isLoading, isError: query.isError }
}

export function TeamSelect({
  teams,
  value,
  onChange,
}: {
  teams: Team[]
  value: string
  onChange: (value: string) => void
}) {
  const selected = teams.find((team) => team.id === value)
  return (
    <Select
      aria-label="Team"
      selectedKey={value || null}
      onSelectionChange={(key) => key !== null && onChange(String(key))}
      className="w-full"
    >
      <Select.Trigger className="h-9 w-full justify-between px-3 text-sm transition-colors">
        <span className="flex min-w-0 items-center gap-2">
          <AppIcon icon="users" className="h-4 w-4 shrink-0 text-muted" />
          <span className="truncate">{selected?.name ?? "Select team"}</span>
        </span>
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover className="p-1.5">
        <ListBox>
          {teams.map((team) => (
            <ListBox.Item
              key={team.id}
              id={team.id ?? ""}
              textValue={team.name ?? ""}
            >
              <span className="flex min-w-0 items-center gap-2">
                <AppIcon icon="users" className="h-4 w-4 shrink-0 text-muted" />
                <span className="truncate text-sm font-medium">
                  {team.name}
                </span>
              </span>
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}
