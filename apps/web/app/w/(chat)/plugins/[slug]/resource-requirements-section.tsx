"use client"

import { useMemo, useState } from "react"
import { Button, Modal, Spinner, toast, useOverlayState } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import type { components } from "@/lib/api/schema"
import { providerLabel } from "@/app/w/(chat)/plugins/[slug]/plugin-detail-helpers"
import {
  pluginResourceDisplayName,
  pluginResourceSelectLabel,
  type PluginResourceRequirement,
} from "@/app/w/(chat)/plugins/_lib"

type AvailableResource = components["schemas"]["AvailableResource"]

export type ResourceModalState = {
  requirement: PluginResourceRequirement
}

export function ResourceRequirementsSection({
  requirements,
  onSelect,
}: {
  requirements: PluginResourceRequirement[]
  onSelect: (requirement: PluginResourceRequirement) => void
}) {
  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-base font-semibold text-foreground">Resources</h2>
      <div className="flex flex-col gap-3">
        {requirements.map((requirement, index) => {
          const provider = requirement.provider ?? ""
          const title = `${provider ? providerLabel(provider) : "Integration"} ${pluginResourceDisplayName(requirement).toLowerCase()} required`
          return (
            <div
              key={`${provider}-${requirement.resource_key || index}`}
              className="border-warning/40 bg-warning/10 flex flex-col gap-3 rounded-xl border p-4 sm:flex-row sm:items-center sm:justify-between"
            >
              <div className="flex min-w-0 items-center gap-3">
                <div className="bg-warning/15 text-warning flex h-10 w-10 shrink-0 items-center justify-center rounded-lg">
                  <AppIcon icon="triangle-alert" className="h-5 w-5" />
                </div>
                <div className="min-w-0">
                  <h3 className="text-sm font-medium text-foreground">
                    {title}
                  </h3>
                  <p className="mt-1 text-sm leading-5 text-muted-foreground">
                    Select resources for this integration before agents use it.
                  </p>
                </div>
              </div>
              <Button
                size="sm"
                variant="primary"
                className="w-full shrink-0 rounded-full sm:w-auto"
                onPress={() => onSelect(requirement)}
              >
                {pluginResourceSelectLabel(requirement)}
              </Button>
            </div>
          )
        })}
      </div>
    </section>
  )
}

export function ResourceSelectionModal({
  modal,
  state,
  onSaved,
  onCancel,
}: {
  modal: ResourceModalState | null
  state: ReturnType<typeof useOverlayState>
  onSaved: () => void
  onCancel: () => void
}) {
  const requirement = modal?.requirement
  if (!requirement) {
    return null
  }

  return (
    <Modal.Root state={state}>
      <Modal.Backdrop className="bg-background/80 backdrop-blur-sm">
        <Modal.Container placement="center" className="p-4">
          <ResourceSelectionModalContent
            key={`${requirement.connection_id ?? ""}:${requirement.resource_key ?? ""}`}
            requirement={requirement}
            onSaved={onSaved}
            onCancel={onCancel}
          />
        </Modal.Container>
      </Modal.Backdrop>
    </Modal.Root>
  )
}

function ResourceSelectionModalContent({
  requirement,
  onSaved,
  onCancel,
}: {
  requirement: PluginResourceRequirement
  onSaved: () => void
  onCancel: () => void
}) {
  const connectionID = requirement.connection_id ?? ""
  const resourceKey = requirement.resource_key ?? ""
  const [selectedIDs, setSelectedIDs] = useState<Set<string>>(new Set())
  const resourcesQuery = $api.useQuery(
    "get",
    "/v1/connections/{id}/resources/{type}",
    {
      params: { path: { id: connectionID, type: resourceKey } },
    },
    { enabled: !!connectionID && !!resourceKey }
  )
  const saveResources = $api.useMutation(
    "put",
    "/v1/connections/{id}/resources"
  )
  const resources = useMemo(
    () => (resourcesQuery.data?.resources ?? []) as AvailableResource[],
    [resourcesQuery.data]
  )

  function toggleResource(resource: AvailableResource) {
    const id = resource.id
    if (!id) return
    setSelectedIDs((current) => {
      const next = new Set(current)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }

  function handleSave() {
    if (!connectionID || !resourceKey) return
    const selected = resources
      .filter((resource) => resource.id && selectedIDs.has(resource.id))
      .map((resource) => resourceSelectionPayload(resource, requirement))
    saveResources.mutate(
      {
        params: { path: { id: connectionID } },
        body: { resources: { [resourceKey]: selected } },
      },
      {
        onSuccess: () => {
          toast.success("Resources selected")
          onSaved()
        },
        onError: (error) =>
          toast.danger(extractErrorMessage(error, "Could not save resources")),
      }
    )
  }

  return (
    <Modal.Dialog className="relative w-full max-w-lg rounded-3xl bg-background p-0 shadow-xl outline-none">
      <div className="flex flex-col gap-5 p-5">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <h2 className="text-base font-semibold text-foreground">
              {pluginResourceSelectLabel(requirement)}
            </h2>
            {requirement.description ? (
              <p className="mt-1 text-sm leading-5 text-muted-foreground">
                {requirement.description}
              </p>
            ) : null}
          </div>
          <button
            type="button"
            aria-label="Close"
            className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-muted/20 hover:text-foreground"
            onClick={onCancel}
          >
            <AppIcon icon="x" className="h-4 w-4" />
          </button>
        </div>

        <div className="max-h-80 overflow-y-auto">
          {resourcesQuery.isLoading ? (
            <div className="flex min-h-32 items-center justify-center">
              <Spinner size="sm" />
            </div>
          ) : resources.length === 0 ? (
            <div className="rounded-xl border border-border bg-card p-4 text-sm text-muted-foreground">
              No resources found for this integration.
            </div>
          ) : (
            <div className="flex flex-col gap-2">
              {resources.map((resource, index) => {
                const id = resource.id ?? ""
                const selected = selectedIDs.has(id)
                return (
                  <button
                    key={id || index}
                    type="button"
                    aria-pressed={selected}
                    className="flex items-center justify-between gap-3 rounded-xl border border-border bg-card px-3 py-2.5 text-left transition-colors hover:bg-muted/20"
                    onClick={() => toggleResource(resource)}
                  >
                    <div className="min-w-0">
                      <p className="truncate text-sm text-foreground">
                        {resource.name || resource.id || "Resource"}
                      </p>
                      {resource.id ? (
                        <p className="truncate text-xs text-muted-foreground">
                          {resource.id}
                        </p>
                      ) : null}
                    </div>
                    <span
                      aria-hidden="true"
                      className={
                        selected
                          ? "flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-success text-success-foreground"
                          : "h-5 w-5 shrink-0 rounded-full border border-border"
                      }
                    >
                      {selected ? (
                        <AppIcon icon="check" className="h-3.5 w-3.5" />
                      ) : null}
                    </span>
                  </button>
                )
              })}
            </div>
          )}
        </div>

        <div className="flex justify-end gap-2">
          <Button size="sm" variant="secondary" onPress={onCancel}>
            Cancel
          </Button>
          <Button
            size="sm"
            variant="primary"
            isDisabled={
              selectedIDs.size === 0 ||
              saveResources.isPending ||
              resourcesQuery.isLoading
            }
            onPress={handleSave}
          >
            {saveResources.isPending ? (
              <Spinner color="current" size="sm" />
            ) : null}
            Save
          </Button>
        </div>
      </div>
    </Modal.Dialog>
  )
}

function resourceSelectionPayload(
  resource: AvailableResource,
  requirement: PluginResourceRequirement
) {
  const id = resource.id ?? ""
  const item: {
    id: string
    name: string
    type?: string
    full_name?: string
  } = {
    id,
    name: resource.name || id,
    type: resource.type || requirement.resource_key,
  }
  if (
    (requirement.provider ?? "").startsWith("github") &&
    requirement.resource_key === "repository"
  ) {
    item.full_name = id
  }
  return item
}
