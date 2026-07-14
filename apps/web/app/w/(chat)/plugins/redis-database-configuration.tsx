"use client"

import { useMemo, useState } from "react"
import { Button, Input, Label } from "@heroui/react"
import {
  type DatabaseConnection,
  type DatabasePolicy,
} from "@/app/w/(chat)/plugins/database-policy-configuration"
import {
  ConfigureFrame,
  EmptyPolicyState,
} from "@/app/w/(chat)/plugins/database-policy-components"

interface RedisKeyInfo {
  key: string
  type?: string
  ttl_seconds?: number
}

export function RedisDatabaseConfiguration({
  connection,
  keys,
  errorMessage,
  introspecting,
  saving,
  canManage = true,
  onRetryIntrospection,
  onSavePolicy,
}: {
  connection: DatabaseConnection | null
  keys: RedisKeyInfo[]
  errorMessage: string | null
  introspecting: boolean
  saving: boolean
  // canManage gates saving/retrying to org admins; the backend enforces
  // this too (all database-integrations mutations are admin-only).
  canManage?: boolean
  onRetryIntrospection: () => void
  onSavePolicy: (policy: DatabasePolicy) => void
}) {
  const initialKeys = useMemo(() => redisPolicyKeys(connection), [connection])
  const [selectedKeys, setSelectedKeys] = useState(initialKeys)
  const [customPattern, setCustomPattern] = useState("")
  const selectedObjectCount = selectedKeys.size
  const canSave = canManage && selectedObjectCount > 0 && !saving

  function addPattern() {
    const value = customPattern.trim()
    if (!value) return
    setSelectedKeys((current) => new Set([...Array.from(current), value]))
    setCustomPattern("")
  }

  return (
    <ConfigureFrame
      selectedObjectCount={selectedObjectCount}
      canSave={canSave}
      saving={saving}
      introspecting={introspecting}
      canManage={canManage}
      errorMessage={errorMessage}
      onRetryIntrospection={onRetryIntrospection}
      onSave={() =>
        onSavePolicy({
          allowed_keys: Array.from(selectedKeys),
        })
      }
    >
      <div className="min-h-0 space-y-4 overflow-y-auto pr-1">
        <div className="space-y-2">
          <Label>Key Patterns</Label>
          <div className="flex gap-2">
            <Input
              value={customPattern}
              onChange={(event) => setCustomPattern(event.target.value)}
              placeholder="user:*"
              onKeyDown={(event) => {
                if (event.key === "Enter") {
                  event.preventDefault()
                  addPattern()
                }
              }}
            />
            <Button
              type="button"
              variant="secondary"
              isDisabled={!customPattern.trim()}
              onPress={addPattern}
            >
              Add
            </Button>
          </div>
        </div>

        {keys.length === 0 ? (
          <EmptyPolicyState text="No keys were found." />
        ) : (
          <div className="space-y-2">
            <Label>Sampled Keys</Label>
            {keys.map((item) => (
              <label
                key={item.key}
                className="flex items-center gap-3 rounded-2xl bg-default px-3 py-3 text-sm"
              >
                <input
                  type="checkbox"
                  checked={selectedKeys.has(item.key)}
                  onChange={() =>
                    setSelectedKeys((current) =>
                      toggleRedisKey(current, item.key)
                    )
                  }
                  className="size-4 accent-current"
                />
                <span className="min-w-0 flex-1 truncate font-medium text-foreground">
                  {item.key}
                </span>
                <span className="shrink-0 text-xs text-muted">
                  {redisKeyDetail(item)}
                </span>
              </label>
            ))}
          </div>
        )}

        {selectedKeys.size > 0 ? (
          <div className="flex flex-wrap gap-2">
            {Array.from(selectedKeys)
              .sort((a, b) => a.localeCompare(b))
              .map((key) => (
                <button
                  key={key}
                  type="button"
                  onClick={() =>
                    setSelectedKeys((current) => toggleRedisKey(current, key))
                  }
                  className="bg-primary text-primary-foreground max-w-full truncate rounded-lg px-3 py-1.5 text-sm"
                >
                  {key}
                </button>
              ))}
          </div>
        ) : null}
      </div>
    </ConfigureFrame>
  )
}

export function normalizeRedisSnapshot(snapshot: unknown): RedisKeyInfo[] {
  if (!Array.isArray(snapshot)) return []
  return snapshot
    .flatMap((item): RedisKeyInfo[] => {
      if (!item || typeof item !== "object" || Array.isArray(item)) return []
      const record = item as Record<string, unknown>
      const key = typeof record.key === "string" ? record.key : ""
      if (!key) return []
      return [
        {
          key,
          type: typeof record.type === "string" ? record.type : "",
          ttl_seconds:
            typeof record.ttl_seconds === "number"
              ? record.ttl_seconds
              : undefined,
        },
      ]
    })
    .sort((a, b) => a.key.localeCompare(b.key))
}

function redisPolicyKeys(connection?: DatabaseConnection | null) {
  const policy =
    connection?.access_policy &&
    typeof connection.access_policy === "object" &&
    !Array.isArray(connection.access_policy)
      ? connection.access_policy
      : {}
  return new Set(
    Array.isArray(policy.allowed_keys)
      ? policy.allowed_keys.filter(
          (item): item is string => typeof item === "string"
        )
      : []
  )
}

function redisKeyDetail(item: RedisKeyInfo) {
  const parts = [item.type].filter(Boolean)
  if (typeof item.ttl_seconds === "number") {
    parts.push(`${item.ttl_seconds}s`)
  }
  return parts.join(" · ") || "key"
}

function toggleRedisKey(values: Set<string>, value: string) {
  const next = new Set(values)
  if (next.has(value)) {
    next.delete(value)
  } else {
    next.add(value)
  }
  return next
}
