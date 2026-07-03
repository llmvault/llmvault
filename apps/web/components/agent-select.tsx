"use client"

import { useMemo, useState } from "react"
import { Picker, PickerButton } from "@/app/w/(chat)/_components/chat-picker"
import { PickerText } from "@/app/w/(chat)/_components/chat-picker-text"
import {
  agentDisplayName,
  type SidebarAgentResponse,
} from "@/app/w/(chat)/_lib/sidebar-data"

export function AgentSelect({
  agents,
  selectedAgentID,
  isLoading,
  onChange,
}: {
  agents: SidebarAgentResponse[]
  selectedAgentID: string
  isLoading: boolean
  onChange: (agentID: string) => void
}) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState("")
  const selected = agents.find((agent) => agent.id === selectedAgentID)
  const filteredAgents = useMemo(() => {
    const term = query.trim().toLowerCase()
    if (!term) return agents
    return agents.filter((entry) =>
      agentDisplayName(entry).toLowerCase().includes(term)
    )
  }, [agents, query])

  return (
    <Picker
      open={open}
      setOpen={(nextOpen) => {
        setOpen(nextOpen)
        if (!nextOpen) setQuery("")
      }}
      label="Select agent"
      icon="bot"
      agent={selected}
      value={
        isLoading
          ? "Loading agents"
          : selected
            ? agentDisplayName(selected)
            : "Select agent"
      }
      width="w-96"
    >
      <input
        type="text"
        value={query}
        onChange={(event) => setQuery(event.target.value)}
        placeholder="Search agents"
        aria-label="Search agents"
        className="mx-1 mt-0.5 mb-1 rounded-lg border border-border bg-transparent px-2 py-1.5 text-sm outline-none placeholder:text-muted focus:border-primary"
      />
      <div className="flex max-h-64 flex-col gap-0.5 overflow-y-auto">
        {isLoading ? (
          <PickerText>Loading agents</PickerText>
        ) : filteredAgents.length ? (
          filteredAgents.map((entry) => (
            <PickerButton
              key={entry.id ?? agentDisplayName(entry)}
              agent={entry}
              selected={entry.id === selectedAgentID}
              description={entry.description?.trim() || undefined}
              onPress={() => {
                if (entry.id) onChange(entry.id)
                setOpen(false)
              }}
            >
              {agentDisplayName(entry)}
            </PickerButton>
          ))
        ) : (
          <PickerText>No matching agents</PickerText>
        )}
      </div>
    </Picker>
  )
}
