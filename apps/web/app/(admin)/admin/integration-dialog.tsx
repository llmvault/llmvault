"use client"

import { useState, type FormEvent } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Button, Modal, toast } from "@heroui/react"
import { Icon } from "@iconify/react"
import {
  adminDataQueryKey,
  deleteAdminIntegration,
  errorMessage,
  upsertAdminIntegration,
} from "./admin-api"
import { Field, Meta } from "./admin-field"
import type { AdminIntegrationDefinition } from "./types"

export function IntegrationDialog({
  adminSecret,
  adminSecretVersion,
  definition,
  onOpenChange,
}: {
  adminSecret: string
  adminSecretVersion: number
  definition: AdminIntegrationDefinition | null
  onOpenChange: (open: boolean) => void
}) {
  const queryClient = useQueryClient()
  const [values, setValues] = useState<Record<string, string>>({})

  const upsertMutation = useMutation({
    mutationFn: ({
      id,
      credentials,
    }: {
      id: string
      credentials?: Record<string, string>
    }) => upsertAdminIntegration(adminSecret, id, credentials),
    onSuccess: async () => {
      toast.success("Integration saved")
      onOpenChange(false)
      await queryClient.invalidateQueries({
        queryKey: adminDataQueryKey(adminSecretVersion),
      })
    },
    onError: (error) => {
      toast.danger(errorMessage(error, "Failed to save integration"))
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteAdminIntegration(adminSecret, id),
    onSuccess: async () => {
      toast.success("Integration removed")
      onOpenChange(false)
      await queryClient.invalidateQueries({
        queryKey: adminDataQueryKey(adminSecretVersion),
      })
    },
    onError: (error) => {
      toast.danger(errorMessage(error, "Failed to remove integration"))
    },
  })

  const fields = definition?.credential_fields ?? []
  const activeConnections = definition?.existing?.active_connections ?? 0
  const canDelete = Boolean(definition?.existing) && activeConnections === 0

  function saveIntegration(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!definition?.id) return
    upsertMutation.mutate({
      id: definition.id,
      credentials: fields.length > 0 ? values : undefined,
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
                    <Icon icon="lucide:x" className="size-4" />
                  </Button>
                </div>

                <div className="bg-surface grid grid-cols-2 gap-3 rounded-2xl border border-border p-3 text-xs">
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
                  <div className="bg-surface rounded-2xl border border-border p-4 text-sm text-muted">
                    This integration does not require editable credentials.
                  </div>
                )}

                <div className="flex flex-col-reverse gap-2 sm:flex-row sm:items-center sm:justify-between">
                  <div className="min-h-9">
                    {definition.existing ? (
                      <span className="inline-flex items-center gap-1.5 text-xs text-muted">
                        <Icon icon="lucide:check-circle" className="size-3.5" />
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
                            deleteMutation.mutate(definition.id)
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
