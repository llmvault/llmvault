"use client"

import { ListBox, Select, Spinner } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import type { components } from "@/lib/api/schema"
import { AgentAvatar } from "@/app/w/settings/agents/_agent-avatar"
import {
  agentDescription,
  agentName,
  type InstalledAgent,
} from "@/app/w/settings/agents/_lib"

type Agent = components["schemas"]["agentListItem"]

export function AgentSelect({
  agents,
  selectedAgentID,
  isLoading,
  onChange,
}: {
  agents: Agent[]
  selectedAgentID: string
  isLoading: boolean
  onChange: (agentID: string) => void
}) {
  const selected = agents.find((agent) => agent.id === selectedAgentID)

  if (isLoading) {
    return <div className="h-9 animate-pulse rounded-md bg-default" />
  }

  if (agents.length === 0) {
    return (
      <div className="text-muted-foreground flex h-9 items-center rounded-md border border-border px-3 text-sm">
        No available agents
      </div>
    )
  }

  return (
    <Select
      aria-label="Agent"
      selectedKey={selectedAgentID || null}
      onSelectionChange={(key) => {
        if (key !== null) onChange(String(key))
      }}
      className="w-full"
    >
      <Select.Trigger className="h-9 w-full justify-between rounded-md px-3 text-sm transition-colors">
        {selected ? (
          <AgentSelectRow agent={selected} compact />
        ) : (
          <span className="text-muted-foreground flex min-w-0 items-center gap-2">
            <AppIcon icon="bot" className="h-4 w-4 shrink-0" />
            <span>Select agent</span>
          </span>
        )}
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover className="rounded-xl p-1.5">
        <ListBox>
          {agents.map((agent) => (
            <ListBox.Item
              key={agent.id}
              id={agent.id ?? ""}
              textValue={agentName(agent)}
            >
              <AgentSelectRow agent={agent} />
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}

function AgentSelectRow({
  agent,
  compact = false,
}: {
  agent: Agent
  compact?: boolean
}) {
  const installedAgent = agent as InstalledAgent

  return (
    <span className="flex min-w-0 items-center gap-2">
      <span
        className={
          compact
            ? "flex h-5 w-5 shrink-0 items-center justify-center overflow-hidden rounded-md [&>*]:scale-75"
            : "flex h-7 w-7 shrink-0 items-center justify-center overflow-hidden rounded-lg [&>*]:scale-90"
        }
      >
        <AgentAvatar agent={installedAgent} size="sm" />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm font-medium text-foreground">
          {agentName(installedAgent)}
        </span>
        {!compact ? (
          <span className="text-muted-foreground block truncate text-xs">
            {agentDescription(installedAgent)}
          </span>
        ) : null}
      </span>
    </span>
  )
}
