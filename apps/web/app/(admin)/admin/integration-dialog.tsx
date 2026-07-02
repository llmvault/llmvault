"use client"

import { useState, type FormEvent } from "react"
import type { QueryClient } from "@tanstack/react-query"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Modal, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { $api } from "@/lib/api/hooks"
import { ADMIN_QUERY_KEYS, adminSecretHeader, errorMessage } from "./admin-api"
import { Field, Meta } from "./admin-field"
import type { AdminCredentialField, AdminIntegrationDefinition } from "./types"

type RawAdminCredentialField = NonNullable<
  AdminIntegrationDefinition["credential_fields"]
>[number]

export function IntegrationDialog({
  adminSecret,
  definition,
  onOpenChange,
}: {
  adminSecret: string
  definition: AdminIntegrationDefinition | null
  onOpenChange: (open: boolean) => void
}) {
  const queryClient = useQueryClient()
  const [values, setValues] = useState<Record<string, string>>({})
  const adminHeaders = adminSecretHeader(adminSecret)

  const upsertMutation = $api.useMutation(
    "put",
    "/v1/admin/integrations/{id}",
    {
      onSuccess: async () => {
        toast.success("Integration saved")
        onOpenChange(false)
        await invalidateAdminQueries(queryClient)
      },
      onError: (error) => {
        toast.danger(errorMessage(error, "Failed to save integration"))
      },
    }
  )

  const deleteMutation = $api.useMutation(
    "delete",
    "/v1/admin/integrations/{id}",
    {
      onSuccess: async () => {
        toast.success("Integration removed")
        onOpenChange(false)
        await invalidateAdminQueries(queryClient)
      },
      onError: (error) => {
        toast.danger(errorMessage(error, "Failed to remove integration"))
      },
    }
  )

  const fields = (definition?.credential_fields ?? []).filter(
    isAdminCredentialField
  )
  const activeConnections = definition?.existing?.active_connections ?? 0
  const canDelete = Boolean(definition?.existing) && activeConnections === 0

  function saveIntegration(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!definition?.id) return
    upsertMutation.mutate({
      params: {
        header: adminHeaders,
        path: { id: definition.id },
      },
      body: {
        credentials: fields.length > 0 ? values : undefined,
      },
    })
  }

  return (
    <Modal isOpen={Boolean(definition)} onOpenChange={onOpenChange}>
      <Modal.Backdrop className="bg-background/80 backdrop-blur-sm">
        <Modal.Container placement="center" className="p-4">
          <Modal.Dialog className="w-full max-w-lg rounded-3xl border border-border bg-background p-0 shadow-xl outline-none">
            {definition ? (
              <form
                onSubmit={saveIntegration}
                className="flex flex-col gap-5 p-6"
              >
                <div className="flex items-start justify-between gap-4">
                  <div>
                    <h2 className="text-xl font-semibold">
                      {definition.display_name ?? "Untitled integration"}
                    </h2>
                    <p className="mt-1 text-sm text-muted">
                      Configure this supported integration for all workspaces.
                    </p>
                  </div>
                  <Button
                    variant="ghost"
                    size="sm"
                    isIconOnly
                    aria-label="Close"
                    onPress={() => onOpenChange(false)}
                  >
                    <AppIcon icon="x" className="size-4" />
                  </Button>
                </div>

                <div className="grid grid-cols-2 gap-3 rounded-2xl border border-border bg-surface p-3 text-xs">
                  <Meta
                    label="Provider"
                    value={definition.provider ?? "unknown"}
                  />
                  <Meta label="Auth" value={definition.auth_mode ?? "none"} />
                  <Meta
                    label="Key"
                    value={definition.unique_key ?? "unknown"}
                  />
                  <Meta label="Connections" value={String(activeConnections)} />
                </div>

                {fields.length > 0 ? (
                  <div className="grid gap-3">
                    {fields.map((field) => (
                      <Field
                        key={field.name}
                        id={`integration-${field.name}`}
                        label={field.label}
                        type={field.secret ? "password" : "text"}
                        value={values[field.name] ?? ""}
                        onChange={(value) =>
                          setValues((current) => ({
                            ...current,
                            [field.name]: value,
                          }))
                        }
                        placeholder={field.placeholder}
                        required={field.required}
                        multiline={field.multiline}
                      />
                    ))}
                  </div>
                ) : (
                  <div className="rounded-2xl border border-border bg-surface p-4 text-sm text-muted">
                    This integration does not require editable credentials.
                  </div>
                )}

                <div className="flex flex-col-reverse gap-2 sm:flex-row sm:items-center sm:justify-between">
                  <div className="min-h-9">
                    {definition.existing ? (
                      <span className="inline-flex items-center gap-1.5 text-xs text-muted">
                        <AppIcon icon="check-circle" className="size-3.5" />
                        Already configured
                      </span>
                    ) : null}
                  </div>
                  <div className="flex justify-end gap-2">
                    {canDelete ? (
                      <Button
                        type="button"
                        variant="danger-soft"
                        isPending={deleteMutation.isPending}
                        onPress={() => {
                          if (definition.id) {
                            deleteMutation.mutate({
                              params: {
                                header: adminHeaders,
                                path: { id: definition.id },
                              },
                            })
                          }
                        }}
                      >
                        Remove
                      </Button>
                    ) : null}
                    <Button
                      type="submit"
                      variant="primary"
                      isPending={upsertMutation.isPending}
                    >
                      Save
                    </Button>
                  </div>
                </div>
              </form>
            ) : null}
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}

async function invalidateAdminQueries(queryClient: QueryClient) {
  await Promise.all(
    Object.values(ADMIN_QUERY_KEYS).map((queryKey) =>
      queryClient.invalidateQueries({ queryKey })
    )
  )
}

function isAdminCredentialField(
  field: RawAdminCredentialField
): field is AdminCredentialField {
  return Boolean(field.name && field.label)
}
