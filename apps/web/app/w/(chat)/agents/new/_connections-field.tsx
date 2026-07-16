"use client"

import { useMemo } from "react"
import { IntegrationLogo } from "@/components/integration-logo"
import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"
import {
  EmptyProvisioningRow,
  ProvisioningRow,
  ProvisioningSkeleton,
  SectionHeader,
} from "@/app/w/settings/teams/[teamId]/_provisioning-row"

type Connection =
  | components["schemas"]["connectionResponse"]
  | components["schemas"]["databaseConnectionResponse"]
type ConnectionMCPToolDeny = components["schemas"]["ConnectionMCPToolDeny"]

const DENY_ALL = "*"

export function AgentConnectionsField({
  teamId,
  value,
  disabled,
  lockedProviders = [],
  onChange,
}: {
  teamId: string
  value: ConnectionMCPToolDeny
  disabled: boolean
  lockedProviders?: string[]
  onChange: (value: ConnectionMCPToolDeny) => void
}) {
  const connectionsQuery = $api.useQuery("get", "/v1/connections", {
    params: { query: { limit: 100 } },
  })
  const databasesQuery = $api.useQuery("get", "/v1/database-integrations")
  const teamConnectionsQuery = $api.useQuery(
    "get",
    "/v1/orgs/current/teams/{teamID}/connections",
    { params: { path: { teamID: teamId } } },
    { enabled: Boolean(teamId) }
  )

  const teamConnectionIDs = useMemo(
    () =>
      new Set(
        (teamConnectionsQuery.data?.data ?? [])
          .map((connection) => connection.id)
          .filter((id): id is string => Boolean(id))
      ),
    [teamConnectionsQuery.data?.data]
  )
  const connections = useMemo<Connection[]>(
    () =>
      [
        ...(connectionsQuery.data?.data ?? []),
        ...(databasesQuery.data ?? []),
      ].filter((connection): connection is Connection =>
        Boolean(connection.id && teamConnectionIDs.has(connection.id))
      ),
    [connectionsQuery.data?.data, databasesQuery.data, teamConnectionIDs]
  )
  const locked = useMemo(() => new Set(lockedProviders), [lockedProviders])
  const loading =
    connectionsQuery.isLoading ||
    databasesQuery.isLoading ||
    teamConnectionsQuery.isLoading

  function toggle(connection: Connection, enabled: boolean) {
    const id = connection.id
    if (!id) return
    const next: ConnectionMCPToolDeny = { ...value }
    const denied = new Set(next[id] ?? [])
    if (enabled) denied.delete(DENY_ALL)
    else denied.add(DENY_ALL)
    if (denied.size === 0) delete next[id]
    else next[id] = Array.from(denied).sort()
    onChange(next)
  }

  return (
    <section className="flex flex-col gap-3">
      <SectionHeader
        title="Connections"
        description="Connections are inherited from the team by default. Toggle access for this agent."
      />
      <div className="overflow-hidden rounded-2xl border border-border bg-surface">
        {!teamId ? (
          <EmptyProvisioningRow text="Select a team to configure connection access." />
        ) : loading ? (
          <ProvisioningSkeleton />
        ) : connections.length === 0 ? (
          <EmptyProvisioningRow text="No connections are granted to this team yet." />
        ) : (
          connections.map((connection, index) => {
            const id = connection.id ?? ""
            const enabled = !(value[id] ?? []).includes(DENY_ALL)
            const provider = connection.provider ?? ""
            const isLocked = locked.has(provider)
            return (
              <ProvisioningRow
                key={id || index}
                last={index === connections.length - 1}
                icon={<IntegrationLogo provider={provider} size={32} />}
                title={
                  connection.name ||
                  connection.display_name ||
                  provider ||
                  "Connection"
                }
                subtitle={provider || "Connection"}
                on={enabled}
                disabled={disabled || !id || isLocked}
                label={`${enabled ? "Disable" : "Enable"} connection for this agent`}
                onChange={(selected) => toggle(connection, selected)}
              />
            )
          })
        )}
      </div>
    </section>
  )
}
