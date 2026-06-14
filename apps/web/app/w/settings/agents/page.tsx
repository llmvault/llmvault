"use client"

import { useRouter } from "next/navigation"
import { Avatar, Button, Chip } from "@heroui/react"
import { Icon } from "@iconify/react"
import { $api } from "@/lib/api/hooks"
import { cn } from "@/lib/utils"
import type { components } from "@/lib/api/schema"

type Agent = components["schemas"]["agentListItem"]

function initials(name: string | undefined) {
  const source = (name ?? "").trim()
  const parts = source.split(/\s+/).filter(Boolean)
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase()
  return (source.slice(0, 2) || "AG").toUpperCase()
}

function enabledToolCount(permissions: unknown) {
  if (!permissions || typeof permissions !== "object") return 0
  return Object.values(permissions as Record<string, unknown>).filter(Boolean)
    .length
}

export default function AgentsSettingsPage() {
  const router = useRouter()
  const agentsQuery = $api.useQuery("get", "/v1/agents")
  const agents = (agentsQuery.data?.data ?? []) as Agent[]

  return (
    <div className="flex flex-col gap-8">
      <div className="flex items-start justify-between gap-4">
        <div className="flex flex-col gap-1">
          <h1 className="text-2xl font-semibold">Agents</h1>
          <p className="text-sm text-muted">
            Create and manage the agents in your workspace.
          </p>
        </div>
        <Button
          variant="primary"
          size="sm"
          onPress={() => router.push("/w/settings/agents/new")}
        >
          <Icon icon="lucide:plus" className="h-4 w-4" />
          Create agent
        </Button>
      </div>

      <div className="overflow-hidden rounded-2xl border border-border bg-surface">
        {agentsQuery.isLoading ? (
          <ListSkeleton />
        ) : agents.length === 0 ? (
          <EmptyState onCreate={() => router.push("/w/settings/agents/new")} />
        ) : (
          agents.map((agent, index) => (
            <AgentRow
              key={agent.id}
              agent={agent}
              last={index === agents.length - 1}
            />
          ))
        )}
      </div>
    </div>
  )
}

function AgentRow({ agent, last }: { agent: Agent; last?: boolean }) {
  const toolCount = enabledToolCount(agent.permissions)
  return (
    <div
      className={cn(
        "flex items-center gap-3 px-4 py-3.5",
        last ? "" : "border-b border-border"
      )}
    >
      <Avatar size="sm" className="shrink-0">
        {agent.avatar_url ? <Avatar.Image src={agent.avatar_url} /> : null}
        <Avatar.Fallback>{initials(agent.name)}</Avatar.Fallback>
      </Avatar>
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="flex items-center gap-2 truncate text-sm font-medium">
          {agent.name}
          {agent.is_default ? (
            <Chip size="sm" color="accent">
              Default
            </Chip>
          ) : null}
        </span>
        <span className="truncate text-sm text-muted">
          {agent.description || "No description"}
        </span>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        {agent.model ? (
          <Chip size="sm" color="default">
            {agent.model}
          </Chip>
        ) : null}
        <span className="hidden text-xs text-muted sm:inline">
          {toolCount} tools
        </span>
      </div>
    </div>
  )
}

function EmptyState({ onCreate }: { onCreate: () => void }) {
  return (
    <div className="flex flex-col items-center gap-4 px-6 py-16 text-center">
      <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-default text-foreground">
        <Icon icon="lucide:bot" className="h-6 w-6" />
      </div>
      <div className="flex flex-col gap-1">
        <p className="text-sm font-medium">No agents yet</p>
        <p className="text-sm text-muted">
          Create your first agent to get started.
        </p>
      </div>
      <Button variant="primary" size="sm" onPress={onCreate}>
        <Icon icon="lucide:plus" className="h-4 w-4" />
        Create agent
      </Button>
    </div>
  )
}

function ListSkeleton() {
  return (
    <div className="flex flex-col">
      {[0, 1, 2].map((index) => (
        <div
          key={index}
          className={cn(
            "flex items-center gap-3 px-4 py-3.5",
            index === 2 ? "" : "border-b border-border"
          )}
        >
          <div className="h-9 w-9 shrink-0 animate-pulse rounded-full bg-default" />
          <div className="flex flex-1 flex-col gap-1.5">
            <div className="h-3.5 w-32 animate-pulse rounded bg-default" />
            <div className="h-3 w-48 animate-pulse rounded bg-default" />
          </div>
        </div>
      ))}
    </div>
  )
}
