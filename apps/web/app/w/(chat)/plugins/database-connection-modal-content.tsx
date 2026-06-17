"use client"

import { useMemo, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Input, Label, Spinner, toast } from "@heroui/react"
import { Icon } from "@iconify/react"
import { IntegrationLogo } from "@/components/integration-logo"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { cn } from "@/lib/utils"
import type { components } from "@/lib/api/schema"

type DatabaseConnection = components["schemas"]["databaseConnectionResponse"]
type DatabasePolicy = components["schemas"]["Policy"]
type DatabaseProvider = "postgres" | "mysql" | "mongodb"

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
}

interface SQLColumn {
  schema: string
  table: string
  column: string
  data_type?: string
}

interface TableNode {
  key: string
  schema: string
  table: string
  columns: SQLColumn[]
}

interface MongoField {
  path: string
  type?: string
}

interface MongoCollection {
  collection: string
  fields: MongoField[]
}

export function isDatabaseProvider(
  provider: string | null | undefined
): provider is DatabaseProvider {
  return (
    provider === "postgres" || provider === "mysql" || provider === "mongodb"
  )
}

export function DatabaseConnectionModalContent({
  provider,
  onConnected,
}: {
  provider: DatabaseProvider
  onConnected: () => void
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
    if (!connectionURL.trim() || isConnecting) return

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
    if (!connection?.id || isConnecting) return
    try {
      await introspect(connection.id)
    } catch (error) {
      setErrorMessage(
        extractErrorMessage(error, "Database introspection failed")
      )
    }
  }

  async function handleSavePolicy(policy: DatabasePolicy) {
    if (!connection?.id || updatePolicy.isPending) return
    try {
      await updatePolicy.mutateAsync({
        params: { path: { id: connection.id } },
        body: policy,
      })
      queryClient.invalidateQueries({
        queryKey: ["get", "/v1/database-integrations"],
      })
      queryClient.invalidateQueries({ queryKey: ["get", "/v1/plugins"] })
      toast.success(`${config.label} connected`)
      onConnected()
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
            <div className="bg-danger/10 text-danger rounded-2xl px-3 py-2 text-sm">
              {errorMessage}
            </div>
          ) : null}

          <Button
            type="submit"
            variant="primary"
            fullWidth
            className="rounded-full"
            isDisabled={!connectionURL.trim() || isConnecting}
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
          onRetryIntrospection={handleRetryIntrospection}
          onSavePolicy={handleSavePolicy}
        />
      )}
    </div>
  )
}

function SQLDatabaseConfiguration({
  connection,
  schemas,
  tables,
  errorMessage,
  introspecting,
  saving,
  onRetryIntrospection,
  onSavePolicy,
}: {
  connection: DatabaseConnection | null
  schemas: string[]
  tables: TableNode[]
  errorMessage: string | null
  introspecting: boolean
  saving: boolean
  onRetryIntrospection: () => void
  onSavePolicy: (policy: DatabasePolicy) => void
}) {
  const initialPolicy = useMemo(
    () => policyFromConnection(connection),
    [connection]
  )
  const [selectedSchemas, setSelectedSchemas] = useState(initialPolicy.schemas)
  const [selectedTables, setSelectedTables] = useState(initialPolicy.tables)
  const [maskedFields, setMaskedFields] = useState(initialPolicy.masks)
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const visibleTables = tables.filter((table) =>
    selectedSchemas.has(table.schema)
  )
  const canSave = selectedTables.size > 0 && !saving

  function toggleSchema(schema: string) {
    setSelectedSchemas((current) => {
      const next = toggleSetValue(current, schema)
      if (!next.has(schema)) {
        setSelectedTables((currentTables) => {
          const nextTables = new Set(currentTables)
          for (const table of tables) {
            if (table.schema === schema) nextTables.delete(table.key)
          }
          return nextTables
        })
      }
      return next
    })
  }

  return (
    <ConfigureFrame
      selectedObjectCount={selectedTables.size}
      canSave={canSave}
      saving={saving}
      introspecting={introspecting}
      errorMessage={errorMessage}
      onRetryIntrospection={onRetryIntrospection}
      onSave={() =>
        onSavePolicy({
          allowed_schemas: Array.from(selectedSchemas),
          allowed_tables: Array.from(selectedTables),
          masked_fields: Array.from(maskedFields),
        })
      }
    >
      <div className="min-h-0 space-y-4 overflow-y-auto pr-1">
        <div className="space-y-2">
          <Label>Schemas</Label>
          <div className="flex flex-wrap gap-2">
            {schemas.map((schema) => (
              <TogglePill
                key={schema}
                selected={selectedSchemas.has(schema)}
                onPress={() => toggleSchema(schema)}
              >
                {schema}
              </TogglePill>
            ))}
          </div>
        </div>

        <div className="space-y-2">
          <Label>Tables</Label>
          {tables.length === 0 ? (
            <EmptyPolicyState text="Select a schema to show its tables." />
          ) : (
            <div className="space-y-2">
              {visibleTables.map((table) => (
                <ExpandablePolicyRow
                  key={table.key}
                  id={table.key}
                  title={table.table}
                  subtitle={table.schema}
                  checked={selectedTables.has(table.key)}
                  expanded={expanded.has(table.key)}
                  onCheckedChange={() =>
                    setSelectedTables((current) =>
                      toggleSetValue(current, table.key)
                    )
                  }
                  onToggleExpanded={() =>
                    setExpanded((current) => toggleSetValue(current, table.key))
                  }
                >
                  <FieldMaskList
                    fields={tableColumns(table).map((column) => ({
                      name: column,
                      detail:
                        table.columns.find((item) => item.column === column)
                          ?.data_type ?? "",
                    }))}
                    disabled={!selectedTables.has(table.key)}
                    maskedFields={maskedFields}
                    onToggleMask={(field) =>
                      setMaskedFields((current) =>
                        toggleSetValue(current, field)
                      )
                    }
                  />
                </ExpandablePolicyRow>
              ))}
            </div>
          )}
        </div>
      </div>
    </ConfigureFrame>
  )
}

function MongoDatabaseConfiguration({
  connection,
  collections,
  errorMessage,
  introspecting,
  saving,
  onRetryIntrospection,
  onSavePolicy,
}: {
  connection: DatabaseConnection | null
  collections: MongoCollection[]
  errorMessage: string | null
  introspecting: boolean
  saving: boolean
  onRetryIntrospection: () => void
  onSavePolicy: (policy: DatabasePolicy) => void
}) {
  const initialPolicy = useMemo(
    () => policyFromConnection(connection),
    [connection]
  )
  const [selectedCollections, setSelectedCollections] = useState(
    initialPolicy.collections
  )
  const [maskedFields, setMaskedFields] = useState(initialPolicy.masks)
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const canSave = selectedCollections.size > 0 && !saving

  return (
    <ConfigureFrame
      selectedObjectCount={selectedCollections.size}
      canSave={canSave}
      saving={saving}
      introspecting={introspecting}
      errorMessage={errorMessage}
      onRetryIntrospection={onRetryIntrospection}
      onSave={() =>
        onSavePolicy({
          allowed_collections: Array.from(selectedCollections),
          masked_fields: Array.from(maskedFields),
        })
      }
    >
      {collections.length === 0 ? (
        <EmptyPolicyState text="No collections were found." />
      ) : (
        <div className="min-h-0 space-y-2 overflow-y-auto pr-1">
          {collections.map((collection) => (
            <ExpandablePolicyRow
              key={collection.collection}
              id={collection.collection}
              title={collection.collection}
              subtitle={`${collection.fields.length} inferred fields`}
              checked={selectedCollections.has(collection.collection)}
              expanded={expanded.has(collection.collection)}
              onCheckedChange={() =>
                setSelectedCollections((current) =>
                  toggleSetValue(current, collection.collection)
                )
              }
              onToggleExpanded={() =>
                setExpanded((current) =>
                  toggleSetValue(current, collection.collection)
                )
              }
            >
              <FieldMaskList
                fields={collection.fields.map((field) => ({
                  name: field.path,
                  detail: field.type ?? "",
                }))}
                disabled={!selectedCollections.has(collection.collection)}
                maskedFields={maskedFields}
                onToggleMask={(field) =>
                  setMaskedFields((current) => toggleSetValue(current, field))
                }
              />
            </ExpandablePolicyRow>
          ))}
        </div>
      )}
    </ConfigureFrame>
  )
}

function ConfigureFrame({
  children,
  selectedObjectCount,
  canSave,
  saving,
  introspecting,
  errorMessage,
  onRetryIntrospection,
  onSave,
}: {
  children: React.ReactNode
  selectedObjectCount: number
  canSave: boolean
  saving: boolean
  introspecting: boolean
  errorMessage: string | null
  onRetryIntrospection: () => void
  onSave: () => void
}) {
  return (
    <div className="flex min-h-0 flex-col gap-4">
      {children}

      {errorMessage ? (
        <div className="bg-danger/10 text-danger flex items-center justify-between gap-3 rounded-2xl px-3 py-2 text-sm">
          <span>{errorMessage}</span>
          <Button
            type="button"
            variant="tertiary"
            size="sm"
            isDisabled={introspecting}
            onPress={onRetryIntrospection}
          >
            {introspecting ? <Spinner color="current" size="sm" /> : null}
            Retry
          </Button>
        </div>
      ) : null}

      <div className="flex items-center justify-between pt-2">
        <p className="text-xs text-muted">
          {selectedObjectCount} item{selectedObjectCount === 1 ? "" : "s"}{" "}
          enabled
        </p>
        <Button
          type="button"
          variant="primary"
          size="sm"
          className="rounded-full"
          isDisabled={!canSave}
          onPress={onSave}
        >
          {saving ? <Spinner color="current" size="sm" /> : null}
          Save access
        </Button>
      </div>
    </div>
  )
}

function TogglePill({
  selected,
  onPress,
  children,
}: {
  selected: boolean
  onPress: () => void
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onPress}
      className={cn(
        "rounded-lg px-3 py-1.5 text-sm transition-colors",
        selected
          ? "bg-primary text-primary-foreground"
          : "bg-default text-muted"
      )}
    >
      {children}
    </button>
  )
}

function ExpandablePolicyRow({
  id,
  title,
  subtitle,
  checked,
  expanded,
  children,
  onCheckedChange,
  onToggleExpanded,
}: {
  id: string
  title: string
  subtitle: string
  checked: boolean
  expanded: boolean
  children: React.ReactNode
  onCheckedChange: () => void
  onToggleExpanded: () => void
}) {
  return (
    <div className="bg-default rounded-2xl">
      <div className="flex items-center gap-3 px-3 py-3">
        <input
          id={`database-object-${id}`}
          type="checkbox"
          checked={checked}
          onChange={onCheckedChange}
          className="size-4 accent-current"
        />
        <Label
          htmlFor={`database-object-${id}`}
          className="min-w-0 flex-1 cursor-pointer"
        >
          <span className="block truncate text-sm font-medium text-foreground">
            {title}
          </span>
          <span className="block truncate text-xs font-normal text-muted">
            {subtitle}
          </span>
        </Label>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          isIconOnly
          onPress={onToggleExpanded}
          aria-label={expanded ? "Collapse fields" : "Expand fields"}
        >
          <Icon
            icon={expanded ? "lucide:chevron-down" : "lucide:chevron-right"}
            className="h-4 w-4"
          />
        </Button>
      </div>
      {expanded ? <div className="px-3 pb-3">{children}</div> : null}
    </div>
  )
}

function FieldMaskList({
  fields,
  disabled,
  maskedFields,
  onToggleMask,
}: {
  fields: { name: string; detail: string }[]
  disabled: boolean
  maskedFields: Set<string>
  onToggleMask: (field: string) => void
}) {
  if (fields.length === 0) {
    return <p className="text-sm text-muted">No fields inferred.</p>
  }

  return (
    <div className="grid gap-2 sm:grid-cols-2">
      {fields.map((field) => (
        <label
          key={field.name}
          className={cn(
            "flex items-center gap-2 rounded-lg px-2 py-1.5 text-sm",
            disabled ? "opacity-50" : "hover:bg-background"
          )}
        >
          <input
            type="checkbox"
            checked={maskedFields.has(field.name)}
            disabled={disabled}
            onChange={() => onToggleMask(field.name)}
            className="size-4 accent-current"
          />
          <span className="min-w-0 flex-1 truncate">{field.name}</span>
          {field.detail ? (
            <span className="shrink-0 text-xs text-muted">{field.detail}</span>
          ) : null}
        </label>
      ))}
    </div>
  )
}

function EmptyPolicyState({ text }: { text: string }) {
  return (
    <div className="bg-default rounded-2xl px-3 py-8 text-center text-sm text-muted">
      {text}
    </div>
  )
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value))
}

function normalizeSQLSnapshot(snapshot: unknown): TableNode[] {
  if (!Array.isArray(snapshot)) return []

  const tables = new Map<string, TableNode>()
  for (const item of snapshot) {
    if (!isRecord(item)) continue
    const schema = String(item.schema ?? "public")
    const table = String(item.table ?? "")
    const column = String(item.column ?? "")
    if (!table || !column) continue

    const key = `${schema}.${table}`
    const node = tables.get(key) ?? { key, schema, table, columns: [] }
    node.columns.push({
      schema,
      table,
      column,
      data_type: typeof item.data_type === "string" ? item.data_type : "",
    })
    tables.set(key, node)
  }

  return Array.from(tables.values()).sort((a, b) => a.key.localeCompare(b.key))
}

function normalizeMongoSnapshot(snapshot: unknown): MongoCollection[] {
  if (!Array.isArray(snapshot)) return []

  return snapshot
    .map((item): MongoCollection | null => {
      if (typeof item === "string") return { collection: item, fields: [] }
      if (!isRecord(item)) return null
      const collection = String(item.collection ?? "")
      if (!collection) return null
      const fields = Array.isArray(item.fields)
        ? item.fields.flatMap((field): MongoField[] => {
            if (!isRecord(field) || typeof field.path !== "string") return []
            return [{ path: field.path, type: String(field.type ?? "") }]
          })
        : []
      return { collection, fields }
    })
    .filter((item): item is MongoCollection => Boolean(item))
    .sort((a, b) => a.collection.localeCompare(b.collection))
}

function policyFromConnection(connection?: DatabaseConnection | null) {
  const policy = isRecord(connection?.access_policy)
    ? connection.access_policy
    : {}
  return {
    schemas: new Set(
      Array.isArray(policy.allowed_schemas)
        ? policy.allowed_schemas.filter(
            (item): item is string => typeof item === "string"
          )
        : []
    ),
    tables: new Set(
      Array.isArray(policy.allowed_tables)
        ? policy.allowed_tables.filter(
            (item): item is string => typeof item === "string"
          )
        : []
    ),
    collections: new Set(
      Array.isArray(policy.allowed_collections)
        ? policy.allowed_collections.filter(
            (item): item is string => typeof item === "string"
          )
        : []
    ),
    masks: new Set(
      Array.isArray(policy.masked_fields)
        ? policy.masked_fields.filter(
            (item): item is string => typeof item === "string"
          )
        : []
    ),
  }
}

function tableColumns(table: TableNode) {
  return table.columns
    .map((column) => column.column)
    .filter((column, index, all) => all.indexOf(column) === index)
    .sort((a, b) => a.localeCompare(b))
}

function toggleSetValue(values: Set<string>, value: string) {
  const next = new Set(values)
  if (next.has(value)) {
    next.delete(value)
  } else {
    next.add(value)
  }
  return next
}
