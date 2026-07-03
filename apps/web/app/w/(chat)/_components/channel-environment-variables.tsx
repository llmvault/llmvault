"use client"

import { type FormEvent, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import {
  AlertDialog,
  Button,
  FieldError,
  Input,
  Label,
  Spinner,
  TextField,
  toast,
} from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"

const ENV_NAME_PATTERN = /^[A-Z_][A-Z0-9_]*$/
const RESERVED_PREFIXES = ["__ENV__", "HIVY_"]

function normalizeEnvName(raw: string) {
  return raw.trim().toUpperCase()
}

function validateEnvName(name: string): string | null {
  if (!name) return "Enter a variable name."
  if (!ENV_NAME_PATTERN.test(name)) {
    return "Use uppercase letters, numbers, and underscores, starting with a letter or underscore."
  }
  if (RESERVED_PREFIXES.some((prefix) => name.startsWith(prefix))) {
    return "Names can't start with __ENV__ or HIVY_."
  }
  return null
}

const ENV_VARS_QUERY_KEY = [
  "get",
  "/v1/channels/{id}/environment-variables",
] as const

/**
 * Environment variables section for the channel details modal. Values are
 * write-only: the API never returns them once saved, so the list only shows
 * masked placeholders. Create/rename/update/delete all rely on the server
 * for authorization (403 surfaces as a toast), matching how the rest of the
 * modal handles management actions.
 */
export function ChannelEnvironmentVariablesPanel({
  channelId,
}: {
  channelId: string
}) {
  const queryClient = useQueryClient()
  const varsQuery = $api.useQuery(
    "get",
    "/v1/channels/{id}/environment-variables",
    { params: { path: { id: channelId } } },
    { enabled: Boolean(channelId), retry: false }
  )
  const createVar = $api.useMutation(
    "post",
    "/v1/channels/{id}/environment-variables"
  )
  const updateVar = $api.useMutation(
    "patch",
    "/v1/channels/{id}/environment-variables/{name}"
  )
  const deleteVar = $api.useMutation(
    "delete",
    "/v1/channels/{id}/environment-variables/{name}"
  )

  const [adding, setAdding] = useState(false)
  const [editingName, setEditingName] = useState<string | null>(null)
  const [deletingName, setDeletingName] = useState<string | null>(null)

  const vars = varsQuery.data?.data ?? []

  function invalidateList() {
    queryClient.invalidateQueries({ queryKey: ENV_VARS_QUERY_KEY })
  }

  return (
    <div className="rounded-xl border border-border bg-surface">
      <div className="flex items-start gap-4 px-5 py-4">
        <div className="min-w-0 flex-1">
          <p className="text-sm font-medium">Environment variables</p>
          <p className="mt-1 text-sm text-muted">
            Injected into sessions started in this channel. Values are
            encrypted and hidden once saved.
          </p>
        </div>
        {!adding ? (
          <Button
            variant="ghost"
            size="sm"
            isDisabled={!channelId}
            onPress={() => {
              setEditingName(null)
              setAdding(true)
            }}
          >
            <AppIcon icon="plus" className="h-4 w-4" />
            Add
          </Button>
        ) : null}
      </div>

      {adding ? (
        <div className="border-t border-border px-5 py-4">
          <AddVariableForm
            pending={createVar.isPending}
            submitError={
              createVar.isError
                ? extractErrorMessage(
                    createVar.error,
                    "Could not add environment variable"
                  )
                : null
            }
            onCancel={() => {
              createVar.reset()
              setAdding(false)
            }}
            onSubmit={(name, value) => {
              createVar.mutate(
                {
                  params: { path: { id: channelId } },
                  body: { name, value },
                },
                {
                  onSuccess: () => {
                    invalidateList()
                    toast.success("Environment variable added")
                    setAdding(false)
                  },
                }
              )
            }}
          />
        </div>
      ) : null}

      {varsQuery.isLoading ? (
        <div className="flex items-center justify-center border-t border-border px-5 py-6">
          <Spinner size="sm" />
        </div>
      ) : null}

      {varsQuery.isError ? (
        <div className="border-t border-border px-5 py-4 text-sm text-danger">
          Could not load environment variables.
        </div>
      ) : null}

      {!varsQuery.isLoading && !varsQuery.isError && vars.length === 0 ? (
        <div className="border-t border-border px-5 py-6 text-sm text-muted">
          No environment variables yet.
        </div>
      ) : null}

      {vars.map((item) => {
        const name = item.name ?? ""
        if (!name) return null
        return (
          <EnvironmentVariableRow
            key={name}
            name={name}
            editing={editingName === name}
            deleting={deletingName === name}
            updatePending={updateVar.isPending}
            deletePending={deleteVar.isPending}
            updateError={
              updateVar.isError && editingName === name
                ? extractErrorMessage(
                    updateVar.error,
                    "Could not update environment variable"
                  )
                : null
            }
            onEdit={() => {
              setAdding(false)
              updateVar.reset()
              setEditingName(name)
            }}
            onCancelEdit={() => {
              updateVar.reset()
              setEditingName(null)
            }}
            onSubmitEdit={(newName, value) => {
              const body: { name?: string; value?: string } = {}
              if (newName !== name) body.name = newName
              if (value) body.value = value
              if (Object.keys(body).length === 0) {
                setEditingName(null)
                return
              }
              updateVar.mutate(
                {
                  params: { path: { id: channelId, name } },
                  body,
                },
                {
                  onSuccess: () => {
                    invalidateList()
                    toast.success("Environment variable updated")
                    setEditingName(null)
                  },
                }
              )
            }}
            onDelete={() => setDeletingName(name)}
            onCancelDelete={() => setDeletingName(null)}
            onConfirmDelete={() => {
              deleteVar.mutate(
                { params: { path: { id: channelId, name } } },
                {
                  onSuccess: () => {
                    invalidateList()
                    toast.success("Environment variable deleted")
                    setDeletingName(null)
                  },
                  onError: (error) =>
                    toast.danger(
                      extractErrorMessage(
                        error,
                        "Could not delete environment variable"
                      )
                    ),
                }
              )
            }}
          />
        )
      })}
    </div>
  )
}

function AddVariableForm({
  onCancel,
  onSubmit,
  pending,
  submitError,
}: {
  onCancel: () => void
  onSubmit: (name: string, value: string) => void
  pending: boolean
  submitError: string | null
}) {
  const [name, setName] = useState("")
  const [value, setValue] = useState("")
  const [touched, setTouched] = useState(false)

  const normalizedName = normalizeEnvName(name)
  const nameError = validateEnvName(normalizedName)
  const valueError = value.length === 0 ? "Enter a value." : null

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setTouched(true)
    if (nameError || valueError || pending) return
    onSubmit(normalizedName, value)
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-3">
      <TextField
        isInvalid={touched && Boolean(nameError)}
        className="flex flex-col gap-1.5"
      >
        <Label>Name</Label>
        <Input
          autoFocus
          placeholder="DATABASE_URL"
          value={name}
          disabled={pending}
          onChange={(event) => setName(event.target.value)}
          className="font-mono"
        />
        <FieldError>{nameError}</FieldError>
      </TextField>
      <TextField
        isInvalid={touched && Boolean(valueError)}
        className="flex flex-col gap-1.5"
      >
        <Label>Value</Label>
        <Input
          type="password"
          placeholder="Value"
          value={value}
          disabled={pending}
          onChange={(event) => setValue(event.target.value)}
        />
        <FieldError>{valueError}</FieldError>
      </TextField>
      {submitError ? (
        <p className="text-sm text-danger">{submitError}</p>
      ) : null}
      <div className="flex justify-end gap-2">
        <Button
          type="button"
          variant="tertiary"
          size="sm"
          isDisabled={pending}
          onPress={onCancel}
        >
          Cancel
        </Button>
        <Button type="submit" variant="primary" size="sm" isDisabled={pending}>
          {pending ? <Spinner color="current" size="sm" /> : null}
          Add
        </Button>
      </div>
    </form>
  )
}

function EnvironmentVariableRow({
  deleting,
  deletePending,
  editing,
  name,
  updateError,
  updatePending,
  onCancelDelete,
  onCancelEdit,
  onConfirmDelete,
  onDelete,
  onEdit,
  onSubmitEdit,
}: {
  deleting: boolean
  deletePending: boolean
  editing: boolean
  name: string
  updateError: string | null
  updatePending: boolean
  onCancelDelete: () => void
  onCancelEdit: () => void
  onConfirmDelete: () => void
  onDelete: () => void
  onEdit: () => void
  onSubmitEdit: (newName: string, value: string) => void
}) {
  return (
    <div className="border-t border-border px-5 py-4">
      {editing ? (
        <EditVariableForm
          name={name}
          pending={updatePending}
          submitError={updateError}
          onCancel={onCancelEdit}
          onSubmit={onSubmitEdit}
        />
      ) : (
        <div className="flex items-center gap-3">
          <div className="min-w-0 flex-1">
            <p className="truncate font-mono text-sm font-medium">{name}</p>
            <p className="mt-1 font-mono text-sm text-muted">••••••••</p>
          </div>
          <Button
            variant="ghost"
            size="sm"
            isIconOnly
            aria-label={`Edit ${name}`}
            onPress={onEdit}
          >
            <AppIcon icon="pencil" className="h-4 w-4 text-muted" />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            isIconOnly
            aria-label={`Delete ${name}`}
            onPress={onDelete}
          >
            <AppIcon icon="trash-2" className="h-4 w-4 text-muted" />
          </Button>
        </div>
      )}

      {deleting ? (
        <AlertDialog>
          <AlertDialog.Backdrop
            isOpen
            onOpenChange={(open) => {
              if (!open && !deletePending) onCancelDelete()
            }}
            className="bg-background/80 backdrop-blur-sm"
          >
            <AlertDialog.Container placement="center" size="sm">
              <AlertDialog.Dialog className="p-8">
                <AlertDialog.Header>
                  <AlertDialog.Icon status="danger">
                    <AppIcon icon="trash-2" className="h-6 w-6" />
                  </AlertDialog.Icon>
                  <div className="flex flex-col gap-1">
                    <AlertDialog.Heading>
                      Delete “{name}”
                    </AlertDialog.Heading>
                    <p className="text-sm text-muted">
                      Sessions started in this channel will no longer receive
                      this variable. This can&apos;t be undone.
                    </p>
                  </div>
                </AlertDialog.Header>
                <AlertDialog.Footer>
                  <Button
                    variant="tertiary"
                    size="sm"
                    isDisabled={deletePending}
                    onPress={onCancelDelete}
                  >
                    Cancel
                  </Button>
                  <Button
                    variant="danger"
                    size="sm"
                    isDisabled={deletePending}
                    onPress={onConfirmDelete}
                  >
                    {deletePending ? (
                      <Spinner color="current" size="sm" />
                    ) : null}
                    Delete
                  </Button>
                </AlertDialog.Footer>
              </AlertDialog.Dialog>
            </AlertDialog.Container>
          </AlertDialog.Backdrop>
        </AlertDialog>
      ) : null}
    </div>
  )
}

function EditVariableForm({
  name,
  onCancel,
  onSubmit,
  pending,
  submitError,
}: {
  name: string
  onCancel: () => void
  onSubmit: (newName: string, value: string) => void
  pending: boolean
  submitError: string | null
}) {
  const [nameDraft, setNameDraft] = useState(name)
  const [value, setValue] = useState("")
  const [touched, setTouched] = useState(false)

  const normalizedName = normalizeEnvName(nameDraft)
  const nameError = validateEnvName(normalizedName)
  const unchanged = normalizedName === name && value.length === 0

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setTouched(true)
    if (nameError || unchanged || pending) return
    onSubmit(normalizedName, value)
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-3">
      <TextField
        isInvalid={touched && Boolean(nameError)}
        className="flex flex-col gap-1.5"
      >
        <Label>Name</Label>
        <Input
          autoFocus
          value={nameDraft}
          disabled={pending}
          onChange={(event) => setNameDraft(event.target.value)}
          className="font-mono"
        />
        <FieldError>{nameError}</FieldError>
      </TextField>
      <TextField className="flex flex-col gap-1.5">
        <Label>Value</Label>
        <Input
          type="password"
          placeholder="Leave blank to keep the current value"
          value={value}
          disabled={pending}
          onChange={(event) => setValue(event.target.value)}
        />
      </TextField>
      {submitError ? (
        <p className="text-sm text-danger">{submitError}</p>
      ) : null}
      <div className="flex justify-end gap-2">
        <Button
          type="button"
          variant="tertiary"
          size="sm"
          isDisabled={pending}
          onPress={onCancel}
        >
          Cancel
        </Button>
        <Button
          type="submit"
          variant="primary"
          size="sm"
          isDisabled={pending || unchanged}
        >
          {pending ? <Spinner color="current" size="sm" /> : null}
          Save
        </Button>
      </div>
    </form>
  )
}
