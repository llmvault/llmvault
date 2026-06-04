"use client"

import { useMemo, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ArrowDown01Icon,
  ArrowLeft01Icon,
  ArrowRight01Icon,
  Loading03Icon,
} from "@hugeicons/core-free-icons"
import { IntegrationLogo } from "@/components/integration-logo"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { cn } from "@/lib/utils"
import type { components } from "@/lib/api/schema"

export type DatabaseProvider = "postgres" | "mysql" | "mongodb"

type DatabaseConnection = components["schemas"]["databaseConnectionResponse"]
type DatabasePolicy = components["schemas"]["Policy"]

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

interface DatabaseConnectionDialogProps {
  provider: DatabaseProvider
  connection?: DatabaseConnection | null
  onBack: () => void
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

  return Array.from(tables.values()).sort((a, b) =>
    a.key.localeCompare(b.key)
  )
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
        ? policy.allowed_schemas.filter((item): item is string => typeof item === "string")
        : []
    ),
    tables: new Set(
      Array.isArray(policy.allowed_tables)
        ? policy.allowed_tables.filter((item): item is string => typeof item === "string")
        : []
    ),
    collections: new Set(
      Array.isArray(policy.allowed_collections)
        ? policy.allowed_collections.filter((item): item is string => typeof item === "string")
        : []
    ),
    masks: new Set(
      Array.isArray(policy.masked_fields)
        ? policy.masked_fields.filter((item): item is string => typeof item === "string")
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

export function DatabaseConnectionDialog({
  provider,
  connection: initialConnection,
  onBack,
}: DatabaseConnectionDialogProps) {
  const queryClient = useQueryClient()
  const config = PROVIDERS[provider]
  const [connection, setConnection] = useState(initialConnection ?? null)
  const [snapshot, setSnapshot] = useState<unknown>(
    initialConnection?.schema_snapshot
  )
  const [step, setStep] = useState<"connect" | "configure">(
    initialConnection ? "configure" : "connect"
  )
  const [connectionURL, setConnectionURL] = useState("")
  const [introspectionError, setIntrospectionError] = useState<string | null>(
    initialConnection && !initialConnection.schema_snapshot
      ? "Schema introspection has not completed."
      : null
  )
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
    setIntrospectionError(null)
  }

  async function handleConnect(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!connectionURL.trim() || isConnecting) return

    setIntrospectionError(null)
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
      setIntrospectionError(
        extractErrorMessage(error, `Failed to connect ${config.label}`)
      )
    }
  }

  async function handleRetryIntrospection() {
    if (!connection?.id || isConnecting) return
    try {
      await introspect(connection.id)
    } catch (error) {
      setIntrospectionError(
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
      toast.success(`${config.label} connected`)
      onBack()
    } catch (error) {
      toast.error(
        extractErrorMessage(error, `Failed to save ${config.label} policy`)
      )
    }
  }

  return (
    <div className="flex max-h-[82vh] flex-col overflow-hidden">
      <DatabaseDialogHeader
        provider={provider}
        label={config.label}
        onBack={onBack}
      />

      {step === "connect" ? (
        <form onSubmit={handleConnect} className="flex flex-col gap-5">
          <div>
            <h3 className="font-heading text-xl font-medium text-foreground">
              Connect {config.label}
            </h3>
            <p className="mt-2 text-sm leading-6 text-muted-foreground">
              Add the database URL. Hivy stores it encrypted and only exposes
              proxy access to the employee runtime.
            </p>
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="database-url">{config.urlLabel}</Label>
            <Input
              id="database-url"
              type="password"
              value={connectionURL}
              onChange={(event) => setConnectionURL(event.target.value)}
              placeholder={config.urlPlaceholder}
              required
              autoFocus
            />
            <p className="text-xs leading-5 text-muted-foreground">
              Use a read-only user or replica whenever possible.
            </p>
          </div>

          {introspectionError ? (
            <div className="rounded-md border border-destructive/25 bg-destructive/5 px-3 py-2 text-sm text-destructive">
              {introspectionError}
            </div>
          ) : null}

          <Button
            type="submit"
            className="w-full"
            disabled={!connectionURL.trim() || isConnecting}
          >
            {isConnecting ? (
              <>
                <HugeiconsIcon
                  icon={Loading03Icon}
                  className="size-4 animate-spin"
                />
                Connecting...
              </>
            ) : (
              "Continue"
            )}
          </Button>
        </form>
      ) : (
        <DatabaseConfigurationStep
          provider={provider}
          connection={connection}
          schemas={schemas}
          tables={sqlTables}
          collections={mongoCollections}
          introspectionError={introspectionError}
          introspecting={introspectConnection.isPending}
          saving={updatePolicy.isPending}
          onRetryIntrospection={handleRetryIntrospection}
          onSavePolicy={handleSavePolicy}
        />
      )}
    </div>
  )
}

function DatabaseConfigurationStep({
  provider,
  connection,
  schemas,
  tables,
  collections,
  introspectionError,
  introspecting,
  saving,
  onRetryIntrospection,
  onSavePolicy,
}: {
  provider: DatabaseProvider
  connection: DatabaseConnection | null
  schemas: string[]
  tables: TableNode[]
  collections: MongoCollection[]
  introspectionError: string | null
  introspecting: boolean
  saving: boolean
  onRetryIntrospection: () => void
  onSavePolicy: (policy: DatabasePolicy) => void
}) {
  if (provider === "mongodb") {
    return (
      <MongoDatabaseConfiguration
        connection={connection}
        collections={collections}
        introspectionError={introspectionError}
        introspecting={introspecting}
        saving={saving}
        onRetryIntrospection={onRetryIntrospection}
        onSavePolicy={onSavePolicy}
      />
    )
  }

  return (
    <SQLDatabaseConfiguration
      connection={connection}
      schemas={schemas}
      tables={tables}
      introspectionError={introspectionError}
      introspecting={introspecting}
      saving={saving}
      onRetryIntrospection={onRetryIntrospection}
      onSavePolicy={onSavePolicy}
    />
  )
}

function SQLDatabaseConfiguration({
  connection,
  schemas,
  tables,
  introspectionError,
  introspecting,
  saving,
  onRetryIntrospection,
  onSavePolicy,
}: {
  connection: DatabaseConnection | null
  schemas: string[]
  tables: TableNode[]
  introspectionError: string | null
  introspecting: boolean
  saving: boolean
  onRetryIntrospection: () => void
  onSavePolicy: (policy: DatabasePolicy) => void
}) {
  const initialPolicy = useMemo(() => policyFromConnection(connection), [connection])
  const [selectedSchemas, setSelectedSchemas] = useState(initialPolicy.schemas)
  const [selectedTables, setSelectedTables] = useState(initialPolicy.tables)
  const [maskedFields, setMaskedFields] = useState(initialPolicy.masks)
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const visibleTables = tables.filter((table) => selectedSchemas.has(table.schema))
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

  function savePolicy() {
    onSavePolicy({
      allowed_schemas: Array.from(selectedSchemas),
      allowed_tables: Array.from(selectedTables),
      masked_fields: Array.from(maskedFields),
    })
  }

  return (
    <ConfigureFrame
      selectedObjectCount={selectedTables.size}
      canSave={canSave}
      saving={saving}
      introspecting={introspecting}
      introspectionError={introspectionError}
      onRetryIntrospection={onRetryIntrospection}
      onSave={savePolicy}
    >
      <SQLPolicyPicker
        schemas={schemas}
        tables={visibleTables}
        selectedSchemas={selectedSchemas}
        selectedTables={selectedTables}
        maskedFields={maskedFields}
        expanded={expanded}
        onToggleSchema={toggleSchema}
        onToggleTable={(table) =>
          setSelectedTables((current) => toggleSetValue(current, table))
        }
        onToggleExpanded={(key) =>
          setExpanded((current) => toggleSetValue(current, key))
        }
        onToggleMask={(field) =>
          setMaskedFields((current) => toggleSetValue(current, field))
        }
      />
    </ConfigureFrame>
  )
}

function MongoDatabaseConfiguration({
  connection,
  collections,
  introspectionError,
  introspecting,
  saving,
  onRetryIntrospection,
  onSavePolicy,
}: {
  connection: DatabaseConnection | null
  collections: MongoCollection[]
  introspectionError: string | null
  introspecting: boolean
  saving: boolean
  onRetryIntrospection: () => void
  onSavePolicy: (policy: DatabasePolicy) => void
}) {
  const initialPolicy = useMemo(() => policyFromConnection(connection), [connection])
  const [selectedCollections, setSelectedCollections] = useState(
    initialPolicy.collections
  )
  const [maskedFields, setMaskedFields] = useState(initialPolicy.masks)
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const canSave = selectedCollections.size > 0 && !saving

  function savePolicy() {
    onSavePolicy({
      allowed_collections: Array.from(selectedCollections),
      masked_fields: Array.from(maskedFields),
    })
  }

  return (
    <ConfigureFrame
      selectedObjectCount={selectedCollections.size}
      canSave={canSave}
      saving={saving}
      introspecting={introspecting}
      introspectionError={introspectionError}
      onRetryIntrospection={onRetryIntrospection}
      onSave={savePolicy}
    >
      <MongoPolicyPicker
        collections={collections}
        selectedCollections={selectedCollections}
        maskedFields={maskedFields}
        expanded={expanded}
        onToggleCollection={(collection) =>
          setSelectedCollections((current) =>
            toggleSetValue(current, collection)
          )
        }
        onToggleExpanded={(collection) =>
          setExpanded((current) => toggleSetValue(current, collection))
        }
        onToggleMask={(field) =>
          setMaskedFields((current) => toggleSetValue(current, field))
        }
      />
    </ConfigureFrame>
  )
}

function ConfigureFrame({
  children,
  selectedObjectCount,
  canSave,
  saving,
  introspecting,
  introspectionError,
  onRetryIntrospection,
  onSave,
}: {
  children: React.ReactNode
  selectedObjectCount: number
  canSave: boolean
  saving: boolean
  introspecting: boolean
  introspectionError: string | null
  onRetryIntrospection: () => void
  onSave: () => void
}) {
  return (
    <div className="flex min-h-0 flex-col gap-4">
      <div>
        <h3 className="font-heading text-xl font-medium text-foreground">
          Choose what Hivy can read
        </h3>
        <p className="mt-2 text-sm leading-6 text-muted-foreground">
          Enable only the data the employee should access. Expand items to mask
          sensitive fields.
        </p>
      </div>

      {children}

      {introspectionError ? (
        <div className="flex items-center justify-between gap-3 rounded-md border border-destructive/25 bg-destructive/5 px-3 py-2 text-sm text-destructive">
          <span>{introspectionError}</span>
          <Button
            type="button"
            variant="secondary"
            size="sm"
            loading={introspecting}
            onClick={onRetryIntrospection}
          >
            Retry
          </Button>
        </div>
      ) : null}

      <div className="flex items-center justify-between border-t border-border pt-4">
        <p className="text-xs text-muted-foreground">
          {selectedObjectCount} item{selectedObjectCount === 1 ? "" : "s"}{" "}
          enabled
        </p>
        <Button
          type="button"
          disabled={!canSave}
          loading={saving}
          onClick={onSave}
        >
          Save access
        </Button>
      </div>
    </div>
  )
}

function DatabaseDialogHeader({
  provider,
  label,
  onBack,
}: {
  provider: DatabaseProvider
  label: string
  onBack: () => void
}) {
  return (
    <div className="mb-5 flex items-center gap-2">
      <button
        type="button"
        onClick={onBack}
        className="-ml-1 flex h-7 w-7 cursor-pointer items-center justify-center rounded-md transition-colors hover:bg-muted"
        aria-label="Back"
      >
        <HugeiconsIcon
          icon={ArrowLeft01Icon}
          size={16}
          className="text-muted-foreground"
        />
      </button>
      <IntegrationLogo provider={provider} size={20} />
      <span className="text-sm font-medium text-muted-foreground">{label}</span>
    </div>
  )
}

function SQLPolicyPicker({
  schemas,
  tables,
  selectedSchemas,
  selectedTables,
  maskedFields,
  expanded,
  onToggleSchema,
  onToggleTable,
  onToggleExpanded,
  onToggleMask,
}: {
  schemas: string[]
  tables: TableNode[]
  selectedSchemas: Set<string>
  selectedTables: Set<string>
  maskedFields: Set<string>
  expanded: Set<string>
  onToggleSchema: (schema: string) => void
  onToggleTable: (table: string) => void
  onToggleExpanded: (key: string) => void
  onToggleMask: (field: string) => void
}) {
  return (
    <div className="min-h-0 space-y-4 overflow-y-auto pr-1">
      <div className="space-y-2">
        <Label>Schemas</Label>
        <div className="flex flex-wrap gap-2">
          {schemas.map((schema) => (
            <button
              key={schema}
              type="button"
              onClick={() => onToggleSchema(schema)}
              className={cn(
                "rounded-md border px-3 py-1.5 text-sm transition-colors",
                selectedSchemas.has(schema)
                  ? "border-primary bg-primary text-primary-foreground"
                  : "border-border bg-card text-foreground hover:bg-muted"
              )}
            >
              {schema}
            </button>
          ))}
        </div>
      </div>

      <div className="space-y-2">
        <Label>Tables</Label>
        {tables.length === 0 ? (
          <EmptyPolicyState text="Select a schema to show its tables." />
        ) : (
          <div className="space-y-2">
            {tables.map((table) => (
              <ExpandablePolicyRow
                key={table.key}
                id={table.key}
                title={table.table}
                subtitle={table.schema}
                checked={selectedTables.has(table.key)}
                expanded={expanded.has(table.key)}
                onCheckedChange={() => onToggleTable(table.key)}
                onToggleExpanded={() => onToggleExpanded(table.key)}
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
                  onToggleMask={onToggleMask}
                />
              </ExpandablePolicyRow>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

function MongoPolicyPicker({
  collections,
  selectedCollections,
  maskedFields,
  expanded,
  onToggleCollection,
  onToggleExpanded,
  onToggleMask,
}: {
  collections: MongoCollection[]
  selectedCollections: Set<string>
  maskedFields: Set<string>
  expanded: Set<string>
  onToggleCollection: (collection: string) => void
  onToggleExpanded: (collection: string) => void
  onToggleMask: (field: string) => void
}) {
  if (collections.length === 0) {
    return <EmptyPolicyState text="No collections were found." />
  }

  return (
    <div className="min-h-0 space-y-2 overflow-y-auto pr-1">
      {collections.map((collection) => (
        <ExpandablePolicyRow
          key={collection.collection}
          id={collection.collection}
          title={collection.collection}
          subtitle={`${collection.fields.length} inferred fields`}
          checked={selectedCollections.has(collection.collection)}
          expanded={expanded.has(collection.collection)}
          onCheckedChange={() => onToggleCollection(collection.collection)}
          onToggleExpanded={() => onToggleExpanded(collection.collection)}
        >
          <FieldMaskList
            fields={collection.fields.map((field) => ({
              name: field.path,
              detail: field.type ?? "",
            }))}
            disabled={!selectedCollections.has(collection.collection)}
            maskedFields={maskedFields}
            onToggleMask={onToggleMask}
          />
        </ExpandablePolicyRow>
      ))}
    </div>
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
    <div className="rounded-md border border-border bg-card">
      <div className="flex items-center gap-3 px-3 py-3">
        <Checkbox id={`database-object-${id}`} checked={checked} onCheckedChange={onCheckedChange} />
        <Label
          htmlFor={`database-object-${id}`}
          className="min-w-0 flex-1 cursor-pointer"
        >
          <span className="block truncate text-sm font-medium text-foreground">
            {title}
          </span>
          <span className="block truncate text-xs font-normal text-muted-foreground">
            {subtitle}
          </span>
        </Label>
        <button
          type="button"
          onClick={onToggleExpanded}
          className="flex size-8 shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground"
          aria-label={expanded ? "Collapse fields" : "Expand fields"}
        >
          <HugeiconsIcon
            icon={expanded ? ArrowDown01Icon : ArrowRight01Icon}
            className="size-4"
          />
        </button>
      </div>
      {expanded ? <div className="border-t border-border px-3 py-3">{children}</div> : null}
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
    return <p className="text-sm text-muted-foreground">No fields inferred.</p>
  }

  return (
    <div className="grid gap-2 sm:grid-cols-2">
      {fields.map((field) => (
        <label
          key={field.name}
          className={cn(
            "flex items-center gap-2 rounded-md px-2 py-1.5 text-sm",
            disabled ? "opacity-50" : "hover:bg-muted"
          )}
        >
          <Checkbox
            checked={maskedFields.has(field.name)}
            disabled={disabled}
            onCheckedChange={() => onToggleMask(field.name)}
          />
          <span className="min-w-0 flex-1 truncate">{field.name}</span>
          {field.detail ? (
            <span className="shrink-0 text-xs text-muted-foreground">
              {field.detail}
            </span>
          ) : null}
        </label>
      ))}
    </div>
  )
}

function EmptyPolicyState({ text }: { text: string }) {
  return (
    <div className="rounded-md border border-dashed border-border px-3 py-8 text-center text-sm text-muted-foreground">
      {text}
    </div>
  )
}
