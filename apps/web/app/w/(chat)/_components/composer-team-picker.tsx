"use client"

import { useMemo, useState } from "react"
import type { components } from "@/lib/api/schema"
import { Picker, PickerButton } from "./chat-picker"
import { PickerText } from "./chat-picker-text"

export type ComposerTeam = components["schemas"]["teamResponse"]

function teamDisplayName(team: ComposerTeam): string {
  return team.name?.trim() || "Untitled team"
}

export function ComposerTeamPicker({
  team,
  teams,
  teamsLoading,
  teamsError,
  onTeamChange,
}: {
  team?: ComposerTeam
  teams: ComposerTeam[]
  teamsLoading: boolean
  teamsError: boolean
  onTeamChange?: (team: ComposerTeam) => void
}) {
  const [teamOpen, setTeamOpen] = useState(false)
  const [teamQuery, setTeamQuery] = useState("")
  const filteredTeams = useMemo(() => {
    const query = teamQuery.trim().toLowerCase()
    if (!query) return teams
    return teams.filter((entry) =>
      teamDisplayName(entry).toLowerCase().includes(query)
    )
  }, [teams, teamQuery])

  const disabled = !teamsLoading && teams.length <= 1

  return (
    <Picker
      open={disabled ? false : teamOpen}
      setOpen={(open) => {
        if (disabled) return
        setTeamOpen(open)
        if (!open) setTeamQuery("")
      }}
      disabled={disabled}
      label="Select team"
      icon="users"
      value={
        teamsLoading
          ? "Loading teams"
          : team
            ? teamDisplayName(team)
            : "Select team"
      }
      width="w-64"
    >
      <input
        type="text"
        value={teamQuery}
        onChange={(event) => setTeamQuery(event.target.value)}
        placeholder="Search teams"
        aria-label="Search teams"
        className="mx-1 mt-0.5 mb-1 rounded-lg border border-border bg-transparent px-2 py-1.5 text-sm outline-none placeholder:text-muted focus:border-primary"
      />
      <div className="flex max-h-64 flex-col gap-0.5 overflow-y-auto">
        {teamsLoading ? (
          <PickerText>Loading teams</PickerText>
        ) : teamsError ? (
          <PickerText>Could not load teams</PickerText>
        ) : filteredTeams.length ? (
          filteredTeams.map((entry) => (
            <PickerButton
              key={entry.id ?? teamDisplayName(entry)}
              icon="users"
              selected={entry.id === team?.id}
              onPress={() => {
                onTeamChange?.(entry)
                setTeamOpen(false)
              }}
            >
              {teamDisplayName(entry)}
            </PickerButton>
          ))
        ) : (
          <PickerText>No matching teams</PickerText>
        )}
      </div>
    </Picker>
  )
}
