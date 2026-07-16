"use client"

import { useMemo, useState } from "react"
import { useRouter } from "next/navigation"
import { Button, Input, Skeleton, toast } from "@heroui/react"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"
import { useIsAdmin } from "@/lib/auth/use-role"
import {
  ConnectionNameModal,
  type ConnectionNameTarget,
} from "./connection-name-modal"
import { ConnectionInventoryRow } from "./connection-inventory-row"
import { DatabasePolicyModal } from "./database-policy-modal"
import { useConnectIntegration } from "./use-connect-integration"

type Connection = components["schemas"]["connectionResponse"]
type DatabaseConnection = components["schemas"]["databaseConnectionResponse"]
type DisconnectTarget =
  | { kind: "integration"; connection: Connection }
  | { kind: "database"; connection: DatabaseConnection }

const EMPTY_CONNECTIONS: Connection[] = []
const EMPTY_DATABASES: DatabaseConnection[] = []

const PROVIDER_NAMES: Record<string, string> = {
  postgres: "PostgreSQL",
  mysql: "MySQL",
  mongodb: "MongoDB",
  redis: "Redis",
  slack: "Slack",
  "github-app": "GitHub",
  "github-app-code-reviews": "GitHub Code Reviews",
  notion: "Notion",
  linear: "Linear",
  railway: "Railway",
  vercel: "Vercel",
  apify: "Apify",
  bugsink: "Bugsink",
  glitchtip: "GlitchTip",
}

export default function ConnectionsPage() {
  const router = useRouter()
  const isAdmin = useIsAdmin()
  const [query, setQuery] = useState("")
  const [nameTarget, setNameTarget] = useState<ConnectionNameTarget | null>(
    null
  )
  const [policyTarget, setPolicyTarget] = useState<DatabaseConnection | null>(
    null
  )
  const [disconnectTarget, setDisconnectTarget] =
    useState<DisconnectTarget | null>(null)

  const connectionsQuery = $api.useQuery("get", "/v1/connections", {
    params: { query: { limit: 100 } },
  })
  const databasesQuery = $api.useQuery("get", "/v1/database-integrations")
  const disconnectIntegration = $api.useMutation(
    "delete",
    "/v1/connections/{id}"
  )
  const disconnectDatabase = $api.useMutation(
    "delete",
    "/v1/database-integrations/{id}"
  )
  const { reconnectIntegration, connectingId } = useConnectIntegration()

  const connections = connectionsQuery.data?.data ?? EMPTY_CONNECTIONS
  const databases = databasesQuery.data ?? EMPTY_DATABASES
  const filteredConnections = useMemo(
    () => filterConnections(connections, query),
    [connections, query]
  )
  const filteredDatabases = useMemo(
    () => filterConnections(databases, query),
    [databases, query]
  )
  const loading = connectionsQuery.isLoading || databasesQuery.isLoading
  const hasError = connectionsQuery.isError || databasesQuery.isError
  const disconnectPending =
    disconnectIntegration.isPending || disconnectDatabase.isPending

  function refresh() {
    void connectionsQuery.refetch()
    void databasesQuery.refetch()
  }

  function rename(
    kind: "integration" | "database",
    connection: Connection | DatabaseConnection
  ) {
    if (!connection.id) return
    setNameTarget({
      id: connection.id,
      kind,
      name: connection.name ?? connection.slug ?? "",
      needsName: connection.needs_name === true,
    })
  }

  function reconnect(connection: Connection) {
    if (!connection.id) return
    reconnectIntegration(connection.id, {
      onSuccess: () => {
        toast.success(`${providerName(connection.provider)} reconnected`)
        refresh()
      },
    })
  }

  async function disconnect() {
    const target = disconnectTarget
    if (!target?.connection.id || disconnectPending) return

    try {
      if (target.kind === "database") {
        await disconnectDatabase.mutateAsync({
          params: { path: { id: target.connection.id } },
        })
      } else {
        await disconnectIntegration.mutateAsync({
          params: { path: { id: target.connection.id } },
        })
      }
      toast.success(`${providerName(target.connection.provider)} disconnected`)
      setDisconnectTarget(null)
      refresh()
    } catch (error) {
      toast.danger(
        extractErrorMessage(error, "Could not disconnect connection")
      )
    }
  }

  return (
    <div className="flex flex-col gap-8">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-lg font-semibold text-foreground">Connections</h1>
          <p className="text-muted-foreground mt-1 text-sm">
            Manage the tools your teams and agents can use.
          </p>
        </div>
        {isAdmin ? (
          <Button
            variant="primary"
            size="sm"
            className="shrink-0"
            onPress={() => router.push("/w/connections/new")}
          >
            <AppIcon icon="plus" className="h-4 w-4" />
            Add connection
          </Button>
        ) : null}
      </div>

      <div className="relative min-w-0 flex-1">
        <AppIcon
          icon="search"
          className="text-muted-foreground pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2"
        />
        <Input
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          placeholder="Search connections"
          aria-label="Search connections"
          className="bg-card h-10 w-full rounded-md pl-9"
        />
      </div>

      {loading ? (
        <InventorySkeleton />
      ) : hasError ? (
        <ErrorState />
      ) : (
        <>
          <ConnectionSection
            title="Connected"
            connections={filteredConnections}
            canManage={isAdmin}
            connectingId={connectingId}
            onRename={(connection) => rename("integration", connection)}
            onReconnect={reconnect}
            onDisconnect={(connection) =>
              setDisconnectTarget({ kind: "integration", connection })
            }
          />
          <DatabaseSection
            databases={filteredDatabases}
            canManage={isAdmin}
            onConfigure={setPolicyTarget}
            onRename={(connection) => rename("database", connection)}
            onDisconnect={(connection) =>
              setDisconnectTarget({ kind: "database", connection })
            }
          />
          {filteredConnections.length === 0 &&
          filteredDatabases.length === 0 ? (
            <EmptyState
              hasConnections={connections.length + databases.length > 0}
            />
          ) : null}
        </>
      )}

      {nameTarget ? (
        <ConnectionNameModal
          key={`${nameTarget.kind}:${nameTarget.id}:${nameTarget.name}`}
          target={nameTarget}
          onClose={() => setNameTarget(null)}
          onSaved={refresh}
        />
      ) : null}

      {policyTarget ? (
        <DatabasePolicyModal
          key={policyTarget.id}
          connection={policyTarget}
          onClose={() => setPolicyTarget(null)}
          onSaved={refresh}
        />
      ) : null}

      <ConfirmDialog
        open={disconnectTarget !== null}
        pending={disconnectPending}
        heading={`Disconnect ${providerName(disconnectTarget?.connection.provider)}`}
        description={
          disconnectTarget?.kind === "database"
            ? "This revokes the stored database credentials and access policy. Agents will lose access until it is connected again."
            : "This revokes the integration connection and removes provider access. Agents will lose access until it is connected again."
        }
        confirmLabel="Disconnect"
        icon="unlink"
        onOpenChange={(open) => !open && setDisconnectTarget(null)}
        onConfirm={disconnect}
      />
    </div>
  )
}

function ConnectionSection({
  title,
  connections,
  canManage,
  connectingId,
  onRename,
  onReconnect,
  onDisconnect,
}: {
  title: string
  connections: Connection[]
  canManage: boolean
  connectingId: string | null
  onRename: (connection: Connection) => void
  onReconnect: (connection: Connection) => void
  onDisconnect: (connection: Connection) => void
}) {
  if (connections.length === 0) return null

  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-sm font-medium text-foreground">{title}</h2>
      <div className="bg-card flex flex-col">
        {connections.map((connection, index) => (
          <ConnectionInventoryRow
            key={
              connection.id ?? `${connection.provider ?? "connection"}-${index}`
            }
            provider={connection.provider ?? ""}
            name={
              connection.name ||
              connection.slug ||
              providerName(connection.provider)
            }
            description={
              connection.display_name ||
              `${providerName(connection.provider)} connection`
            }
            needsName={connection.needs_name === true}
            canManage={canManage && connectingId !== connection.id}
            onRename={() => onRename(connection)}
            onReconnect={() => onReconnect(connection)}
            onDisconnect={() => onDisconnect(connection)}
          />
        ))}
      </div>
    </section>
  )
}

function DatabaseSection({
  databases,
  canManage,
  onConfigure,
  onRename,
  onDisconnect,
}: {
  databases: DatabaseConnection[]
  canManage: boolean
  onConfigure: (connection: DatabaseConnection) => void
  onRename: (connection: DatabaseConnection) => void
  onDisconnect: (connection: DatabaseConnection) => void
}) {
  if (databases.length === 0) return null

  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-sm font-medium text-foreground">Databases</h2>
      <div className="bg-card flex flex-col">
        {databases.map((connection, index) => (
          <ConnectionInventoryRow
            key={
              connection.id ?? `${connection.provider ?? "database"}-${index}`
            }
            provider={connection.provider ?? ""}
            name={
              connection.name ||
              connection.slug ||
              providerName(connection.provider)
            }
            description={`${providerName(connection.provider)} database`}
            needsName={connection.needs_name === true}
            canManage={canManage}
            onConfigure={() => onConfigure(connection)}
            onRename={() => onRename(connection)}
            onDisconnect={() => onDisconnect(connection)}
          />
        ))}
      </div>
    </section>
  )
}

function filterConnections<T extends Connection | DatabaseConnection>(
  values: T[],
  query: string
): T[] {
  const normalized = query.trim().toLowerCase()
  if (!normalized) return values

  return values.filter((connection) =>
    `${connection.name ?? ""} ${connection.slug ?? ""} ${connection.display_name ?? ""} ${connection.provider ?? ""} ${providerName(connection.provider)}`
      .toLowerCase()
      .includes(normalized)
  )
}

function providerName(provider: string | null | undefined): string {
  if (!provider) return "connection"
  return PROVIDER_NAMES[provider] ?? provider
}

function InventorySkeleton() {
  return (
    <section className="flex flex-col gap-3">
      <Skeleton className="h-4 w-24 rounded" />
      <div className="bg-card flex flex-col gap-3">
        {Array.from({ length: 4 }).map((_, index) => (
          <div key={index} className="flex items-center gap-3 py-1.5">
            <Skeleton className="h-9 w-9" />
            <div className="min-w-0 flex-1">
              <Skeleton className="h-4 w-40 rounded" />
              <Skeleton className="mt-2 h-4 w-28 rounded" />
            </div>
          </div>
        ))}
      </div>
    </section>
  )
}

function ErrorState() {
  return (
    <div className="bg-card flex min-h-56 flex-col items-center justify-center rounded-xl px-6 text-center">
      <AppIcon
        icon="triangle-alert"
        className="text-muted-foreground h-7 w-7"
      />
      <p className="mt-3 text-sm font-medium text-foreground">
        Could not load connections
      </p>
      <p className="text-muted-foreground mt-1 text-sm">
        Refresh the page to try again.
      </p>
    </div>
  )
}

function EmptyState({ hasConnections }: { hasConnections: boolean }) {
  return (
    <div className="bg-card flex min-h-56 flex-col items-center justify-center rounded-xl px-6 text-center">
      <AppIcon icon="plug" className="text-muted-foreground h-7 w-7" />
      <p className="mt-3 text-sm font-medium text-foreground">
        {hasConnections ? "No matching connections" : "No connections yet"}
      </p>
      <p className="text-muted-foreground mt-1 text-sm">
        {hasConnections
          ? "Try a different search."
          : "Add a connection to give agents access to your tools."}
      </p>
    </div>
  )
}
