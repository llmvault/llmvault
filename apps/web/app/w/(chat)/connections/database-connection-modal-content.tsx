"use client"

import { useMemo, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Input, Label, Spinner, toast } from "@heroui/react"
import { IntegrationLogo } from "@/components/integration-logo"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { queryKeys } from "@/lib/api/query-keys"
import {
  MongoDatabaseConfiguration,
  SQLDatabaseConfiguration,
  normalizeMongoSnapshot,
  normalizeSQLSnapshot,
  type DatabaseConnection,
  type DatabasePolicy,
} from "@/app/w/(chat)/connections/database-policy-configuration"
import {
  RedisDatabaseConfiguration,
  normalizeRedisSnapshot,
} from "@/app/w/(chat)/connections/redis-database-configuration"

export type DatabaseProvider = "postgres" | "mysql" | "mongodb" | "redis"

interface ProviderConfig {
  label: string
  urlLabel: string
  urlPlaceholder: string
}

const PROVIDERS: Record<DatabaseProvider, ProviderConfig> = {
  postgres: {
    label: "PostgreSQL",
    urlLabel: "Connection URL",
    urlPlaceholder:
      "postgres://readonly:password@host:5432/database?sslmode=require",
  },
  mysql: {
    label: "MySQL",
    urlLabel: "Connection URL",
    urlPlaceholder: "user:password@tcp(host:3306)/database?tls=true",
  },
  mongodb: {
    label: "MongoDB",
    urlLabel: "Connection URI",
    urlPlaceholder: "mongodb+srv://readonly:password@cluster/database",
  },
  redis: {
    label: "Redis",
    urlLabel: "Connection URL",
    urlPlaceholder: "redis://default:password@host:6379/0",
  },
}

export function isDatabaseProvider(
  provider: string | null | undefined
): provider is DatabaseProvider {
  return (
    provider === "postgres" ||
    provider === "mysql" ||
    provider === "mongodb" ||
    provider === "redis"
  )
}

export function DatabaseConnectionModalContent({
  provider,
  onConnected,
  canManage = true,
}: {
  provider: DatabaseProvider
  onConnected: (connection: DatabaseConnection) => void
  // canManage gates connecting/introspecting/saving a database connection to
  // org admins; the backend enforces this too (all database-integrations
  // mutations are admin-only).
  canManage?: boolean
}) {
  const queryClient = useQueryClient()
  const config = PROVIDERS[provider]
  const [connection, setConnection] = useState<DatabaseConnection | null>(null)
  const [snapshot, setSnapshot] = useState<unknown>(null)
  const [step, setStep] = useState<"connect" | "configure">("connect")
  const [connectionURL, setConnectionURL] = useState("")
  const [errorMessage, setErrorMessage] = useState<string | null>(null)

  const createConnection = $api.useMutation("post", "/v1/database-integrations")
  const introspectConnection = $api.useMutation(
    "post",
    "/v1/database-integrations/{id}/introspect"
  )
  const updatePolicy = $api.useMutation(
    "put",
    "/v1/database-integrations/{id}/policy"
  )

  const sqlTables = useMemo(() => normalizeSQLSnapshot(snapshot), [snapshot])
  const mongoCollections = useMemo(
    () => normalizeMongoSnapshot(snapshot),
    [snapshot]
  )
  const redisKeys = useMemo(() => normalizeRedisSnapshot(snapshot), [snapshot])
  const schemas = useMemo(() => {
    return Array.from(new Set(sqlTables.map((table) => table.schema))).sort(
      (a, b) => a.localeCompare(b)
    )
  }, [sqlTables])

  const isConnecting =
    createConnection.isPending || introspectConnection.isPending

  async function introspect(connectionID: string) {
    const updatedConnection = await introspectConnection.mutateAsync({
      params: { path: { id: connectionID } },
    })
    setConnection(updatedConnection ?? null)
    setSnapshot(updatedConnection?.schema_snapshot)
    setStep("configure")
    setErrorMessage(null)
  }

  async function handleConnect(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!canManage || !connectionURL.trim() || isConnecting) return

    setErrorMessage(null)
    try {
      const created = await createConnection.mutateAsync({
        body: {
          provider,
          display_name: config.label,
          connection_url: connectionURL.trim(),
        },
      })
      setConnection(created ?? null)
      if (!created?.id) throw new Error("Database connection was not created")
      await introspect(created.id)
    } catch (error) {
      setErrorMessage(
        extractErrorMessage(error, `Failed to connect ${config.label}`)
      )
    }
  }

  async function handleRetryIntrospection() {
    if (!canManage || !connection?.id || isConnecting) return
    try {
      await introspect(connection.id)
    } catch (error) {
      setErrorMessage(
        extractErrorMessage(error, "Database introspection failed")
      )
    }
  }

  async function handleSavePolicy(policy: DatabasePolicy) {
    if (!canManage || !connection?.id || updatePolicy.isPending) return
    try {
      await updatePolicy.mutateAsync({
        params: { path: { id: connection.id } },
        body: policy,
      })
      queryClient.invalidateQueries({
        queryKey: queryKeys.databaseIntegrations(),
      })
      toast.success(`${config.label} connected`)
      onConnected(connection)
    } catch (error) {
      toast.danger(
        extractErrorMessage(error, `Failed to save ${config.label} policy`)
      )
    }
  }

  return (
    <div className="flex max-h-[82vh] flex-col overflow-hidden p-6">
      <div className="mb-6 flex items-center gap-3">
        <IntegrationLogo provider={provider} size={28} />
        <div className="min-w-0">
          <h2 className="text-lg font-semibold text-foreground">
            Connect {config.label}
          </h2>
          <p className="text-sm text-muted">Choose what Hivy can read.</p>
        </div>
      </div>

      {step === "connect" ? (
        <form onSubmit={handleConnect} className="flex flex-col gap-5">
          <label className="flex flex-col gap-2">
            <Label htmlFor="database-url">{config.urlLabel}</Label>
            <Input
              id="database-url"
              type="password"
              value={connectionURL}
              onChange={(event) => setConnectionURL(event.target.value)}
              placeholder={config.urlPlaceholder}
              autoFocus
            />
            <span className="text-xs leading-5 text-muted">
              Use a read-only user or replica whenever possible.
            </span>
          </label>

          {errorMessage ? (
            <div className="rounded-2xl bg-danger/10 px-3 py-2 text-sm text-danger">
              {errorMessage}
            </div>
          ) : null}

          {!canManage ? (
            <p className="text-muted-foreground text-xs leading-5">
              Only workspace admins can connect databases.
            </p>
          ) : null}

          <Button
            type="submit"
            variant="primary"
            fullWidth
            className="rounded-full"
            isDisabled={!canManage || !connectionURL.trim() || isConnecting}
          >
            {isConnecting ? <Spinner color="current" size="sm" /> : null}
            Continue
          </Button>
        </form>
      ) : provider === "mongodb" ? (
        <MongoDatabaseConfiguration
          connection={connection}
          collections={mongoCollections}
          errorMessage={errorMessage}
          introspecting={introspectConnection.isPending}
          saving={updatePolicy.isPending}
          canManage={canManage}
          onRetryIntrospection={handleRetryIntrospection}
          onSavePolicy={handleSavePolicy}
        />
      ) : provider === "redis" ? (
        <RedisDatabaseConfiguration
          connection={connection}
          keys={redisKeys}
          errorMessage={errorMessage}
          introspecting={introspectConnection.isPending}
          saving={updatePolicy.isPending}
          canManage={canManage}
          onRetryIntrospection={handleRetryIntrospection}
          onSavePolicy={handleSavePolicy}
        />
      ) : (
        <SQLDatabaseConfiguration
          connection={connection}
          schemas={schemas}
          tables={sqlTables}
          errorMessage={errorMessage}
          introspecting={introspectConnection.isPending}
          saving={updatePolicy.isPending}
          canManage={canManage}
          onRetryIntrospection={handleRetryIntrospection}
          onSavePolicy={handleSavePolicy}
        />
      )}
    </div>
  )
}
