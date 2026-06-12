"use client"

import { useEffect, useMemo, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { HugeiconsIcon } from "@hugeicons/react"
import { Alert02Icon, Search01Icon } from "@hugeicons/core-free-icons"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { cn } from "@/lib/utils"
import type { components } from "@/lib/api/schema"

type Connection = components["schemas"]["connectionResponse"]
type Employee = components["schemas"]["employeeListItem"]
type ResourceSummary = components["schemas"]["ConfigurableResourceSummary"]

type ResourceItem = {
  id?: string
  name?: string
  type?: string
  full_name?: string
}

type SelectedByType = Record<string, ResourceItem[]>

interface ResourceSelectionDialogProps {
  connection: Connection
  employee: Employee | undefined
  onDone: () => void
}

export function ResourceSelectionDialog({
  connection,
  employee,
  onDone,
}: ResourceSelectionDialogProps) {
  const queryClient = useQueryClient()
  const resources = connection.configurable_resources ?? []
  const [selected, setSelected] = useState<SelectedByType>({})

  useEffect(() => {
    setSelected(selectedResourcesForConnection(employee, connection))
  }, [connection, employee])

  const saveResources = $api.useMutation(
    "put",
    "/v1/employees/{id}/connections/{connectionID}/resources"
  )

  function toggle(resourceType: string, resource: ResourceItem) {
    if (!resource.id || !resource.name) return
    setSelected((current) => {
      const existing = current[resourceType] ?? []
      const present = existing.some((item) => item.id === resource.id)
      const nextItems = present
        ? existing.filter((item) => item.id !== resource.id)
        : [
            ...existing,
            {
              id: resource.id,
              name: resource.name,
              type: resourceType,
              full_name: resource.full_name ?? resource.id,
            },
          ]
      return { ...current, [resourceType]: nextItems }
    })
  }

  function handleSave() {
    if (!employee?.id || !connection.id) return
    saveResources.mutate(
      {
        params: {
          path: {
            id: employee.id,
            connectionID: connection.id,
          },
        },
        body: { resources: selected } as never,
      },
      {
        onSuccess: () => {
          toast.success("Resources saved")
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/employees"] })
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/connections"] })
          onDone()
        },
        onError: (error) => {
          toast.error(extractErrorMessage(error, "Failed to save resources"))
        },
      }
    )
  }

  return (
    <>
      <DialogHeader>
        <DialogTitle>Configure resources</DialogTitle>
        <DialogDescription>
          Select the resources this employee can use from{" "}
          {connection.display_name ?? connection.provider ?? "this connection"}.
        </DialogDescription>
      </DialogHeader>

      <div className="space-y-5">
        {employee ? (
          resources.map((resource) => (
            <ResourceTypePicker
              key={resource.key}
              connection={connection}
              resource={resource}
              selected={selected[resource.key ?? ""] ?? []}
              onToggle={toggle}
            />
          ))
        ) : (
          <div className="rounded-md border border-border p-4">
            <Skeleton className="h-5 w-40" />
            <Skeleton className="mt-3 h-10 w-full" />
          </div>
        )}

        {resources.length === 0 ? (
          <div className="flex items-start gap-2 rounded-md border border-border bg-muted/30 p-3 text-sm text-muted-foreground">
            <HugeiconsIcon icon={Alert02Icon} className="mt-0.5 size-4" />
            This connection does not expose selectable resources.
          </div>
        ) : null}
      </div>

      <DialogFooter>
        <Button type="button" variant="outline" onClick={onDone}>
          Cancel
        </Button>
        <Button
          type="button"
          loading={saveResources.isPending}
          disabled={!employee?.id || resources.length === 0}
          onClick={handleSave}
        >
          Save resources
        </Button>
      </DialogFooter>
    </>
  )
}

function ResourceTypePicker({
  connection,
  resource,
  selected,
  onToggle,
}: {
  connection: Connection
  resource: ResourceSummary
  selected: ResourceItem[]
  onToggle: (resourceType: string, resource: ResourceItem) => void
}) {
  const [query, setQuery] = useState("")
  const resourceType = resource.key ?? ""
  const resourcesQuery = $api.useQuery(
    "get",
    "/v1/connections/{id}/resources/{type}",
    {
      params: {
        path: {
          id: connection.id ?? "",
          type: resourceType,
        },
      },
    },
    { enabled: Boolean(connection.id && resourceType) }
  )

  const selectedIDs = useMemo(
    () => new Set(selected.map((item) => item.id).filter(Boolean)),
    [selected]
  )
  const items = (resourcesQuery.data?.resources ?? []) as ResourceItem[]
  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase()
    if (!needle) return items
    return items.filter((item) => {
      return (
        (item.name ?? "").toLowerCase().includes(needle) ||
        (item.id ?? "").toLowerCase().includes(needle)
      )
    })
  }, [items, query])

  return (
    <section className="rounded-md border border-border">
      <div className="border-b border-border p-4">
        <div className="flex items-start justify-between gap-3">
          <div>
            <h3 className="text-sm font-medium text-foreground">
              {resource.display_name ?? resourceType}
            </h3>
            {resource.description ? (
              <p className="mt-1 text-xs leading-5 text-muted-foreground">
                {resource.description}
              </p>
            ) : null}
          </div>
          <span className="shrink-0 text-xs text-muted-foreground">
            {selected.length} selected
          </span>
        </div>
        <div className="relative mt-3">
          <HugeiconsIcon
            icon={Search01Icon}
            className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder={`Search ${resource.display_name ?? resourceType}`}
            className="h-10 rounded-md bg-card pl-9"
          />
        </div>
      </div>

      <div className="max-h-72 overflow-y-auto p-2">
        {resourcesQuery.isLoading ? (
          <div className="space-y-2 p-2">
            <Skeleton className="h-11 w-full" />
            <Skeleton className="h-11 w-full" />
            <Skeleton className="h-11 w-full" />
          </div>
        ) : null}

        {!resourcesQuery.isLoading && filtered.length === 0 ? (
          <p className="px-3 py-6 text-center text-sm text-muted-foreground">
            No resources found
          </p>
        ) : null}

        {filtered.map((item) => {
          const checked = Boolean(item.id && selectedIDs.has(item.id))
          return (
            <button
              key={item.id}
              type="button"
              className={cn(
                "flex w-full items-center gap-3 rounded-md px-3 py-2.5 text-left transition-colors hover:bg-muted",
                checked && "bg-muted"
              )}
              onClick={() => onToggle(resourceType, item)}
            >
              <Checkbox checked={checked} readOnly />
              <span className="min-w-0">
                <span className="block truncate text-sm font-medium text-foreground">
                  {item.name}
                </span>
                <span className="block truncate text-xs text-muted-foreground">
                  {item.id}
                </span>
              </span>
            </button>
          )
        })}
      </div>
    </section>
  )
}

function selectedResourcesForConnection(
  employee: Employee | undefined,
  connection: Connection
): SelectedByType {
  if (!employee || !connection.id) return {}
  const resources = (employee as Employee & { resources?: Record<string, unknown> })
    .resources
  const raw = resources?.[connection.id]
  if (!raw || typeof raw !== "object") return {}
  return raw as SelectedByType
}
