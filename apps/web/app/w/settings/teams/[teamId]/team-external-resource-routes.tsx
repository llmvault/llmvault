"use client"

import { useMemo, useState } from "react"
import { Button, Spinner, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { useTeamAgents } from "@/lib/api/team-agents"

export function TeamExternalResourceRoutes({ teamId }: { teamId: string }) {
  const routesQuery = $api.useQuery(
    "get",
    "/v1/orgs/current/teams/{id}/external-resource-routes",
    { params: { path: { id: teamId } } }
  )
  const connectionsQuery = $api.useQuery(
    "get",
    "/v1/orgs/current/teams/{teamID}/connections",
    { params: { path: { teamID: teamId } } }
  )
  const allConnectionsQuery = $api.useQuery("get", "/v1/connections", {
    params: { query: { limit: 100 } },
  })
  const { agents, isLoading: agentsLoading } = useTeamAgents(teamId)
  const createRoute = $api.useMutation(
    "post",
    "/v1/orgs/current/teams/{id}/external-resource-routes"
  )
  const deleteRoute = $api.useMutation(
    "delete",
    "/v1/orgs/current/teams/{id}/external-resource-routes/{routeID}"
  )
  const updateRoute = $api.useMutation(
    "patch",
    "/v1/orgs/current/teams/{id}/external-resource-routes/{routeID}"
  )
  const [connectionID, setConnectionID] = useState("")
  const grantedConnectionIDs = useMemo(
    () => new Set((connectionsQuery.data?.data ?? []).map((item) => item.id)),
    [connectionsQuery.data?.data]
  )
  const connections = useMemo(
    () =>
      (allConnectionsQuery.data?.data ?? []).filter((item) =>
        grantedConnectionIDs.has(item.id)
      ),
    [allConnectionsQuery.data?.data, grantedConnectionIDs]
  )
  const connection = connections.find((item) => item.id === connectionID)
  const resourceTypes = connection?.configurable_resources ?? []
  const [resourceType, setResourceType] = useState("")
  const resourcesQuery = $api.useQuery(
    "get",
    "/v1/connections/{id}/resources/{type}",
    { params: { path: { id: connectionID, type: resourceType } } },
    { enabled: Boolean(connectionID && resourceType) }
  )
  const [resourceKey, setResourceKey] = useState("")
  const [agentID, setAgentID] = useState("")
  const selectedAgentID = agentID || agents[0]?.id || ""

  const resources = resourcesQuery.data?.resources ?? []
  const selectedResource = resources.find((item) => item.id === resourceKey)
  const ready = Boolean(
    connectionID && resourceType && resourceKey && selectedAgentID
  )

  function changeConnection(nextConnectionID: string) {
    const nextConnection = connections.find((item) => item.id === nextConnectionID)
    setConnectionID(nextConnectionID)
    setResourceType(nextConnection?.configurable_resources?.[0]?.key ?? "")
    setResourceKey("")
  }

  function changeResourceType(nextResourceType: string) {
    setResourceType(nextResourceType)
    setResourceKey("")
  }

  function refresh() {
    void routesQuery.refetch()
  }

  function create() {
    if (!ready) return
    createRoute.mutate(
      {
        params: { path: { id: teamId } },
        body: {
          connection_id: connectionID,
          agent_id: selectedAgentID,
          resource_type: resourceType,
          resource_key: resourceKey,
          resource_name: selectedResource?.name ?? resourceKey,
        },
      },
      {
        onSuccess: () => {
          toast.success("External resource route created")
          refresh()
          setResourceKey("")
        },
        onError: (error) =>
          toast.danger(
            extractErrorMessage(error, "Could not create external resource route")
          ),
      }
    )
  }

  function remove(routeID: string) {
    deleteRoute.mutate(
      { params: { path: { id: teamId, routeID } } },
      {
        onSuccess: () => {
          toast.success("External resource route removed")
          refresh()
        },
        onError: (error) =>
          toast.danger(
            extractErrorMessage(error, "Could not remove external resource route")
          ),
      }
    )
  }

  function assignAgent(routeID: string, nextAgentID: string) {
    updateRoute.mutate(
      {
        params: { path: { id: teamId, routeID } },
        body: { agent_id: nextAgentID },
      },
      {
        onSuccess: () => {
          toast.success("External resource route updated")
          refresh()
        },
        onError: (error) =>
          toast.danger(
            extractErrorMessage(error, "Could not update external resource route")
          ),
      }
    )
  }

  return (
    <section className="flex flex-col gap-3">
      <div>
        <h2 className="text-sm font-medium">External routing</h2>
        <p className="mt-1 text-sm text-muted">
          Route conversations from a connected provider resource to one agent in
          this team. Existing conversation affinity always takes precedence.
        </p>
      </div>

      <div className="rounded-2xl border border-border bg-surface p-4">
        <div className="grid gap-3 md:grid-cols-2">
          <RouteSelect
            label="Connection"
            value={connectionID}
            onChange={changeConnection}
            disabled={
              connectionsQuery.isLoading ||
              allConnectionsQuery.isLoading ||
              createRoute.isPending
            }
          >
            <option value="">Select a team connection</option>
            {connections.map((item) => (
              <option key={item.id} value={item.id ?? ""}>
                {item.name || item.display_name || item.provider || item.id}
              </option>
            ))}
          </RouteSelect>
          <RouteSelect
            label="Resource type"
            value={resourceType}
            onChange={changeResourceType}
            disabled={!connectionID || createRoute.isPending}
          >
            <option value="">Select a resource type</option>
            {resourceTypes.map((item) => (
              <option key={item.key} value={item.key ?? ""}>
                {item.display_name || item.key}
              </option>
            ))}
          </RouteSelect>
          <RouteSelect
            label="External resource"
            value={resourceKey}
            onChange={setResourceKey}
            disabled={!resourceType || resourcesQuery.isLoading || createRoute.isPending}
          >
            <option value="">
              {resourcesQuery.isLoading
                ? "Loading resources…"
                : "Select a provider resource"}
            </option>
            {resources.map((item) => (
              <option key={item.id} value={item.id ?? ""}>
                {item.name || item.id}
              </option>
            ))}
          </RouteSelect>
          <RouteSelect
            label="Agent"
            value={selectedAgentID}
            onChange={setAgentID}
            disabled={agentsLoading || createRoute.isPending}
          >
            <option value="">Select an agent</option>
            {agents.map((item) => (
              <option key={item.id} value={item.id ?? ""}>
                {item.name || item.id}
              </option>
            ))}
          </RouteSelect>
        </div>
        <div className="mt-4 flex justify-end">
          <Button
            size="sm"
            variant="primary"
            isDisabled={!ready || createRoute.isPending}
            onPress={create}
          >
            {createRoute.isPending ? <Spinner color="current" size="sm" /> : null}
            Add route
          </Button>
        </div>
      </div>

      <div className="overflow-hidden rounded-2xl border border-border bg-surface">
        {routesQuery.isLoading ? (
          <div className="p-4 text-sm text-muted">Loading routes…</div>
        ) : routesQuery.data?.data?.length ? (
          routesQuery.data.data.map((route, index, rows) => {
            const agent = agents.find((item) => item.id === route.agent_id)
            return (
              <div
                key={route.id}
                className={`flex items-center gap-3 px-4 py-3 ${
                  index < rows.length - 1 ? "border-b border-border" : ""
                }`}
              >
                <AppIcon icon="route" className="h-4 w-4 text-muted" />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">
                    {route.resource_name || route.resource_key}
                  </p>
                  <p className="truncate text-xs text-muted">
                    {route.resource_type} → {agent?.name || "Assigned agent"}
                  </p>
                </div>
                <select
                  aria-label={`Agent for ${route.resource_name || route.resource_key}`}
                  value={route.agent_id ?? ""}
                  disabled={updateRoute.isPending || !route.id}
                  onChange={(event) =>
                    route.id && assignAgent(route.id, event.target.value)
                  }
                  className="h-8 max-w-40 rounded-lg border border-border bg-default px-2 text-xs outline-none disabled:cursor-not-allowed disabled:opacity-60"
                >
                  {agents.map((item) => (
                    <option key={item.id} value={item.id ?? ""}>
                      {item.name || item.id}
                    </option>
                  ))}
                </select>
                <Button
                  size="sm"
                  variant="ghost"
                  isIconOnly
                  aria-label={`Remove route for ${route.resource_name || route.resource_key}`}
                  isDisabled={deleteRoute.isPending || !route.id}
                  onPress={() => route.id && remove(route.id)}
                >
                  <AppIcon icon="trash-2" className="h-4 w-4" />
                </Button>
              </div>
            )
          })
        ) : (
          <div className="p-4 text-sm text-muted">
            No external resources are routed to this team yet.
          </div>
        )}
      </div>
    </section>
  )
}

function RouteSelect({
  label,
  value,
  onChange,
  disabled,
  children,
}: {
  label: string
  value: string
  onChange: (value: string) => void
  disabled?: boolean
  children: React.ReactNode
}) {
  return (
    <label className="flex min-w-0 flex-col gap-1.5 text-sm">
      <span className="text-muted">{label}</span>
      <select
        value={value}
        disabled={disabled}
        onChange={(event) => onChange(event.target.value)}
        className="h-9 rounded-lg border border-border bg-default px-2 text-sm outline-none disabled:cursor-not-allowed disabled:opacity-60"
      >
        {children}
      </select>
    </label>
  )
}
