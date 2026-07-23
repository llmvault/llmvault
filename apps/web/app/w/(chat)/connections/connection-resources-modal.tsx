"use client"

import { useMemo, useState } from "react"
import { Button, Input, Modal, Spinner, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { IntegrationLogo } from "@/components/integration-logo"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"
import { cn } from "@/lib/utils"

type Connection = components["schemas"]["connectionResponse"]
type ConfigurableResource =
  components["schemas"]["ConfigurableResourceSummary"]
type AvailableResource = components["schemas"]["AvailableResource"]
type ResourceSelection =
  components["schemas"]["agentConnectionResourceSelection"]
type ResourceSelections = Record<string, ResourceSelection[]>

export function connectionNeedsResourceConfiguration(
  connection: Connection
): boolean {
  const types = configurableResourceTypes(connection)
  if (types.length === 0) return false
  const selected = selectedConnectionResources(connection)
  return types.some((type) => (selected[type.key]?.length ?? 0) === 0)
}

export function ConnectionResourcesModal({
  connection,
  onClose,
  onSaved,
}: {
  connection: Connection
  onClose: () => void
  onSaved: () => void
}) {
  const resourceTypes = useMemo(
    () => configurableResourceTypes(connection),
    [connection]
  )
  const [selections, setSelections] = useState<ResourceSelections>(() =>
    selectedConnectionResources(connection)
  )
  const updateResources = $api.useMutation(
    "put",
    "/v1/connections/{id}/resources"
  )
  const selectedCount = Object.values(selections).reduce(
    (total, items) => total + items.length,
    0
  )
  const connectionID = connection.id ?? ""
  const provider = connection.provider ?? ""
  const connectionName =
    connection.name || connection.display_name || provider || "Connection"

  function updateType(type: string, items: ResourceSelection[]) {
    setSelections((current) => ({ ...current, [type]: items }))
  }

  async function save() {
    if (!connectionID || updateResources.isPending) return
    const resources = Object.fromEntries(
      Object.entries(selections).filter(([, items]) => items.length > 0)
    )
    try {
      await updateResources.mutateAsync({
        params: { path: { id: connectionID } },
        body: { resources },
      })
      toast.success("Connection resources updated")
      onSaved()
      onClose()
    } catch (error) {
      toast.danger(
        extractErrorMessage(error, "Could not update connection resources")
      )
    }
  }

  function close() {
    if (!updateResources.isPending) onClose()
  }

  return (
    <Modal isOpen onOpenChange={(open) => !open && close()}>
      <Modal.Backdrop className="bg-background/80 backdrop-blur-sm">
        <Modal.Container placement="center" className="p-4">
          <Modal.Dialog className="w-full max-w-lg bg-background p-0 shadow-xl outline-none">
            <Modal.CloseTrigger />
            <div className="flex max-h-[74vh] flex-col overflow-hidden">
              <Modal.Header className="flex items-start gap-3 border-b border-border px-5 py-4 pr-12">
                <IntegrationLogo provider={provider} size={32} />
                <div className="min-w-0">
                  <Modal.Heading>Configure resources</Modal.Heading>
                  <p className="mt-1 text-sm text-muted">
                    Choose what agents can access through {connectionName}.
                  </p>
                </div>
              </Modal.Header>

              <Modal.Body className="min-h-0 overflow-y-auto px-5 py-4">
                <div className="flex flex-col gap-5">
                  {resourceTypes.map((resourceType) => (
                    <ConnectionResourcePicker
                      key={resourceType.key}
                      connectionID={connectionID}
                      resourceType={resourceType}
                      value={selections[resourceType.key] ?? []}
                      onChange={(items) =>
                        updateType(resourceType.key, items)
                      }
                    />
                  ))}
                </div>
              </Modal.Body>

              <Modal.Footer className="flex items-center justify-between gap-3 border-t border-border px-5 py-3.5">
                <p className="text-sm text-muted">
                  {selectedCount === 0
                    ? "No resources selected"
                    : `${selectedCount} selected`}
                </p>
                <div className="flex items-center gap-2">
                  <Button
                    type="button"
                    variant="secondary"
                    isDisabled={updateResources.isPending}
                    onPress={close}
                  >
                    Cancel
                  </Button>
                  <Button
                    type="button"
                    variant="primary"
                    isDisabled={!connectionID || updateResources.isPending}
                    onPress={save}
                  >
                    {updateResources.isPending ? (
                      <Spinner size="sm" color="current" />
                    ) : null}
                    Save resources
                  </Button>
                </div>
              </Modal.Footer>
            </div>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}

function ConnectionResourcePicker({
  connectionID,
  resourceType,
  value,
  onChange,
}: {
  connectionID: string
  resourceType: Required<Pick<ConfigurableResource, "key">> &
    ConfigurableResource
  value: ResourceSelection[]
  onChange: (items: ResourceSelection[]) => void
}) {
  const [query, setQuery] = useState("")
  const resourcesQuery = $api.useQuery(
    "get",
    "/v1/connections/{id}/resources/{type}",
    {
      params: {
        path: {
          id: connectionID,
          type: resourceType.key,
        },
      },
    },
    { enabled: Boolean(connectionID), retry: false }
  )
  const options = useMemo(
    () =>
      mergeResources(
        value,
        normalizeAvailableResources(
          resourcesQuery.data?.resources ?? [],
          resourceType.key
        )
      ),
    [resourceType.key, resourcesQuery.data?.resources, value]
  )
  const filtered = useMemo(() => {
    const normalized = query.trim().toLowerCase()
    if (!normalized) return options
    return options.filter((resource) =>
      `${resource.name ?? ""} ${resource.id ?? ""}`
        .toLowerCase()
        .includes(normalized)
    )
  }, [options, query])
  const selectedIDs = useMemo(
    () => new Set(value.map((item) => item.id).filter(Boolean)),
    [value]
  )
  const label =
    resourceType.display_name || resourceType.key || "Provider resources"

  function toggle(resource: ResourceSelection) {
    const id = resource.id
    if (!id) return
    if (selectedIDs.has(id)) {
      onChange(value.filter((item) => item.id !== id))
      return
    }
    onChange([...value, resource])
  }

  return (
    <section className="flex flex-col gap-3">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h3 className="text-sm font-medium text-foreground">{label}</h3>
          {resourceType.description ? (
            <p className="mt-1 text-sm text-muted">
              {resourceType.description}
            </p>
          ) : null}
        </div>
        {value.length > 0 ? (
          <button
            type="button"
            className="shrink-0 text-sm text-muted transition-colors hover:text-foreground"
            onClick={() => onChange([])}
          >
            Clear
          </button>
        ) : null}
      </div>

      {resourcesQuery.isLoading ? (
        <div className="flex min-h-32 items-center justify-center text-muted">
          <Spinner size="sm" color="current" />
        </div>
      ) : resourcesQuery.isError ? (
        <div className="flex min-h-32 flex-col items-center justify-center gap-3 rounded-xl border border-border px-4 text-center">
          <p className="text-sm text-muted">Could not load {label}.</p>
          <Button
            type="button"
            size="sm"
            variant="secondary"
            onPress={() => void resourcesQuery.refetch()}
          >
            Try again
          </Button>
        </div>
      ) : options.length === 0 ? (
        <div className="rounded-xl border border-border px-4 py-8 text-center text-sm text-muted">
          No {label.toLowerCase()} are available.
        </div>
      ) : (
        <>
          <div className="relative">
            <AppIcon
              icon="search"
              className="pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2 text-muted"
            />
            <Input
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              aria-label={`Search ${label}`}
              placeholder={`Search ${label.toLowerCase()}`}
              className="h-9 w-full rounded-md bg-card pl-9"
            />
          </div>
          <div className="max-h-56 overflow-y-auto rounded-xl border border-border">
            {filtered.length === 0 ? (
              <p className="px-4 py-8 text-center text-sm text-muted">
                No matching resources.
              </p>
            ) : (
              filtered.map((resource, index) => {
                const id = resource.id ?? ""
                const selected = selectedIDs.has(id)
                return (
                  <button
                    key={id || index}
                    type="button"
                    aria-pressed={selected}
                    onClick={() => toggle(resource)}
                    className={cn(
                      "flex w-full items-center gap-3 px-3 py-2.5 text-left transition-colors hover:bg-default",
                      index < filtered.length - 1
                        ? "border-b border-border"
                        : ""
                    )}
                  >
                    <AppIcon
                      icon={selected ? "check-circle" : "circle"}
                      className={cn(
                        "h-4 w-4 shrink-0",
                        selected ? "text-primary" : "text-muted"
                      )}
                    />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-medium text-foreground">
                        {resource.name || id}
                      </span>
                      {resource.name && id !== resource.name ? (
                        <span className="block truncate text-xs text-muted">
                          {id}
                        </span>
                      ) : null}
                    </span>
                  </button>
                )
              })
            )}
          </div>
        </>
      )}
    </section>
  )
}

function configurableResourceTypes(
  connection: Connection
): Array<
  Required<Pick<ConfigurableResource, "key">> & ConfigurableResource
> {
  return (connection.configurable_resources ?? [])
    .filter(
      (
        resource
      ): resource is Required<Pick<ConfigurableResource, "key">> &
        ConfigurableResource => Boolean(resource.key)
    )
    .sort((left, right) =>
      (left.display_name || left.key).localeCompare(
        right.display_name || right.key
      )
    )
}

function selectedConnectionResources(
  connection: Connection
): ResourceSelections {
  const meta = asRecord(connection.meta)
  const rawResources = asRecord(meta?.resources)
  if (!rawResources) return {}

  const selected: ResourceSelections = {}
  for (const [type, rawItems] of Object.entries(rawResources)) {
    if (!Array.isArray(rawItems)) continue
    const items = rawItems
      .map((item) => normalizeSelection(item, type))
      .filter((item): item is ResourceSelection => item !== null)
    if (items.length > 0) selected[type] = items
  }
  return selected
}

function normalizeAvailableResources(
  resources: AvailableResource[],
  type: string
): ResourceSelection[] {
  return resources
    .map((resource) => normalizeSelection(resource, type))
    .filter((resource): resource is ResourceSelection => resource !== null)
}

function normalizeSelection(
  value: unknown,
  type: string
): ResourceSelection | null {
  const item = asRecord(value)
  const id = typeof item?.id === "string" ? item.id.trim() : ""
  const name = typeof item?.name === "string" ? item.name.trim() : ""
  if (!id || !name) return null
  const fullName =
    typeof item?.full_name === "string" ? item.full_name.trim() : ""
  return {
    id,
    name,
    type,
    ...(fullName ? { full_name: fullName } : {}),
  }
}

function mergeResources(
  selected: ResourceSelection[],
  discovered: ResourceSelection[]
): ResourceSelection[] {
  const resources = new Map<string, ResourceSelection>()
  for (const resource of [...selected, ...discovered]) {
    if (resource.id && !resources.has(resource.id)) {
      resources.set(resource.id, resource)
    }
  }
  return Array.from(resources.values()).sort((left, right) =>
    (left.name || left.id || "").localeCompare(right.name || right.id || "")
  )
}

function asRecord(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) return null
  return value as Record<string, unknown>
}
