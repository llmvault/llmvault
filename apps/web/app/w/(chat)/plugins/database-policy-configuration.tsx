"use client"

import { useMemo, useState } from "react"
import { Label } from "@heroui/react"
import type { components } from "@/lib/api/schema"
import {
  ConfigureFrame,
  EmptyPolicyState,
  ExpandablePolicyRow,
  FieldMaskList,
  TogglePill,
} from "./database-policy-components"

export type DatabaseConnection =
  components["schemas"]["databaseConnectionResponse"]
export type DatabasePolicy = components["schemas"]["Policy"]

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

export function SQLDatabaseConfiguration({
  connection,
  schemas,
  tables,
  errorMessage,
  introspecting,
  saving,
  canManage = true,
  onRetryIntrospection,
  onSavePolicy,
}: {
  connection: DatabaseConnection | null
  schemas: string[]
  tables: TableNode[]
  errorMessage: string | null
  introspecting: boolean
  saving: boolean
  // canManage gates saving/retrying to org admins; the backend enforces
  // this too (all database-integrations mutations are admin-only).
  canManage?: boolean
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
  const visibleTableKeys = visibleTables.map((table) => table.key)
  const allVisibleTablesSelected =
    visibleTableKeys.length > 0 &&
    visibleTableKeys.every((table) => selectedTables.has(table))
  const canSave = canManage && selectedTables.size > 0 && !saving

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
      canManage={canManage}
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
          <div className="flex items-center justify-between gap-4">
            <Label>Tables</Label>
            <label className="flex cursor-pointer items-center gap-2 text-sm text-muted">
              <input
                type="checkbox"
                checked={allVisibleTablesSelected}
                disabled={visibleTableKeys.length === 0}
                onChange={() =>
                  setSelectedTables((current) =>
                    setAllValues(
                      current,
                      visibleTableKeys,
                      !allVisibleTablesSelected
                    )
                  )
                }
                className="size-4 accent-current"
              />
              Select all
            </label>
          </div>
          {visibleTables.length === 0 ? (
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
                    threeColumns
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

export function MongoDatabaseConfiguration({
  connection,
  collections,
  errorMessage,
  introspecting,
  saving,
  canManage = true,
  onRetryIntrospection,
  onSavePolicy,
}: {
  connection: DatabaseConnection | null
  collections: MongoCollection[]
  errorMessage: string | null
  introspecting: boolean
  saving: boolean
  // canManage gates saving/retrying to org admins; the backend enforces
  // this too (all database-integrations mutations are admin-only).
  canManage?: boolean
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
  const canSave = canManage && selectedCollections.size > 0 && !saving

  return (
    <ConfigureFrame
      selectedObjectCount={selectedCollections.size}
      canSave={canSave}
      saving={saving}
      introspecting={introspecting}
      canManage={canManage}
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

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value))
}

export function normalizeSQLSnapshot(snapshot: unknown): TableNode[] {
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

export function normalizeMongoSnapshot(snapshot: unknown): MongoCollection[] {
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

export function setAllValues(
  values: Set<string>,
  selectableValues: string[],
  selected: boolean
) {
  const next = new Set(values)
  for (const value of selectableValues) {
    if (selected) {
      next.add(value)
    } else {
      next.delete(value)
    }
  }
  return next
}
