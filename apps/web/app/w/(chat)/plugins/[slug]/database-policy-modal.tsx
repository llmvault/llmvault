"use client"

import { useMemo, useState } from "react"
import { Modal, toast } from "@heroui/react"
import { useQueryClient } from "@tanstack/react-query"
import { IntegrationLogo } from "@/components/integration-logo"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { queryKeys } from "@/lib/api/query-keys"
import {
  isDatabaseProvider,
  type DatabaseProvider,
} from "@/app/w/(chat)/plugins/database-connection-modal-content"
import {
  MongoDatabaseConfiguration,
  SQLDatabaseConfiguration,
  normalizeMongoSnapshot,
  normalizeSQLSnapshot,
  policyForUpdate,
  type DatabaseConnection,
  type DatabasePolicy,
} from "@/app/w/(chat)/plugins/database-policy-configuration"
import {
  RedisDatabaseConfiguration,
  normalizeRedisSnapshot,
} from "@/app/w/(chat)/plugins/redis-database-configuration"

const PROVIDER_LABELS: Record<DatabaseProvider, string> = {
  postgres: "PostgreSQL",
  mysql: "MySQL",
  mongodb: "MongoDB",
  redis: "Redis",
}

export function DatabasePolicyModal({
  connection,
  onClose,
  onSaved,
}: {
  connection: DatabaseConnection
  onClose: () => void
  onSaved: () => void
}) {
  const queryClient = useQueryClient()
  const [current, setCurrent] = useState(connection)
  const [snapshot, setSnapshot] = useState<unknown>(connection.schema_snapshot)
  const [errorMessage, setErrorMessage] = useState<string | null>(null)
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
  const schemas = useMemo(
    () =>
      Array.from(new Set(sqlTables.map((table) => table.schema))).sort((a, b) =>
        a.localeCompare(b)
      ),
    [sqlTables]
  )

  if (!isDatabaseProvider(current.provider)) return null

  const provider = current.provider
  const label = PROVIDER_LABELS[provider]

  async function handleRetryIntrospection() {
    if (!current.id || introspectConnection.isPending) return
    try {
      const updated = await introspectConnection.mutateAsync({
        params: { path: { id: current.id } },
      })
      if (updated) {
        setCurrent(updated)
        setSnapshot(updated.schema_snapshot)
      }
      setErrorMessage(null)
    } catch (error) {
      setErrorMessage(
        extractErrorMessage(error, "Database introspection failed")
      )
    }
  }

  async function handleSavePolicy(policy: DatabasePolicy) {
    if (!current.id || updatePolicy.isPending) return
    try {
      await updatePolicy.mutateAsync({
        params: { path: { id: current.id } },
        body: policyForUpdate(current, policy),
      })
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: queryKeys.databaseIntegrations(),
        }),
        queryClient.invalidateQueries({ queryKey: queryKeys.plugins() }),
      ])
      toast.success(`${label} access updated`)
      onSaved()
      onClose()
    } catch (error) {
      toast.danger(
        extractErrorMessage(error, `Could not update ${label} access`)
      )
    }
  }

  const configurationProps = {
    connection: current,
    errorMessage,
    introspecting: introspectConnection.isPending,
    saving: updatePolicy.isPending,
    onRetryIntrospection: handleRetryIntrospection,
    onSavePolicy: handleSavePolicy,
  }

  return (
    <Modal isOpen onOpenChange={(open) => !open && onClose()}>
      <Modal.Backdrop className="bg-background/80 backdrop-blur-sm">
        <Modal.Container placement="center" className="p-4">
          <Modal.Dialog className="w-full max-w-3xl bg-background p-0 shadow-xl outline-none">
            <Modal.CloseTrigger />
            <div className="flex max-h-[82vh] flex-col overflow-hidden p-6">
              <div className="mb-6 flex items-center gap-3 pr-8">
                <IntegrationLogo provider={provider} size={28} />
                <div className="min-w-0">
                  <Modal.Heading>Configure {label} access</Modal.Heading>
                  <p className="mt-1 text-sm text-muted">
                    Choose the data agents can read from this connection.
                  </p>
                </div>
              </div>

              {provider === "mongodb" ? (
                <MongoDatabaseConfiguration
                  {...configurationProps}
                  collections={mongoCollections}
                />
              ) : provider === "redis" ? (
                <RedisDatabaseConfiguration
                  {...configurationProps}
                  keys={redisKeys}
                />
              ) : (
                <SQLDatabaseConfiguration
                  {...configurationProps}
                  schemas={schemas}
                  tables={sqlTables}
                />
              )}
            </div>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}
