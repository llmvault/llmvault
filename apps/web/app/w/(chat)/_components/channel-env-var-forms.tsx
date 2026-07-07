"use client"

import { type FormEvent, useState } from "react"
import {
  AlertDialog,
  Button,
  FieldError,
  Input,
  Label,
  Popover,
  Skeleton,
  Spinner,
  TextArea,
  TextField,
} from "@heroui/react"
import { AppIcon } from "@/components/icon"

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

function SecretValueInput({
  value,
  onChange,
  disabled,
  placeholder,
}: {
  value: string
  onChange: (value: string) => void
  disabled: boolean
  placeholder: string
}) {
  const [show, setShow] = useState(false)
  return (
    <div className="relative w-full">
      <Input
        type={show ? "text" : "password"}
        placeholder={placeholder}
        value={value}
        disabled={disabled}
        onChange={(event) => onChange(event.target.value)}
        className="w-full pr-10"
      />
      <button
        type="button"
        aria-label={show ? "Hide value" : "Show value"}
        disabled={disabled}
        onClick={() => setShow((prev) => !prev)}
        className="text-muted-foreground absolute inset-y-0 right-0 flex items-center px-3 transition-colors hover:text-foreground disabled:opacity-50"
      >
        <AppIcon icon={show ? "eye-off" : "eye"} className="h-4 w-4" />
      </button>
    </div>
  )
}

export function EnvVarsSkeleton() {
  return (
    <div className="flex flex-col gap-2">
      {Array.from({ length: 3 }).map((_, index) => (
        <div
          key={index}
          className="flex items-center gap-3 rounded-xl border border-border bg-surface px-4 py-3.5"
        >
          <div className="min-w-0 flex-1 flex-col gap-2">
            <Skeleton className="h-4 w-40 rounded" />
            <Skeleton className="mt-2 h-4 w-20 rounded" />
          </div>
          <Skeleton className="h-8 w-8 rounded-md" />
          <Skeleton className="h-8 w-8 rounded-md" />
        </div>
      ))}
    </div>
  )
}

export function AddVariableForm({
  onCancel,
  onSubmit,
  pending,
  submitError,
}: {
  onCancel: () => void
  onSubmit: (name: string, value: string, description: string) => void
  pending: boolean
  submitError: string | null
}) {
  const [name, setName] = useState("")
  const [value, setValue] = useState("")
  const [description, setDescription] = useState("")
  const [touched, setTouched] = useState(false)

  const normalizedName = normalizeEnvName(name)
  const nameError = validateEnvName(normalizedName)
  const valueError = value.length === 0 ? "Enter a value." : null

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setTouched(true)
    if (nameError || valueError || pending) return
    onSubmit(normalizedName, value, description.trim())
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
        <SecretValueInput
          placeholder="Value"
          value={value}
          disabled={pending}
          onChange={setValue}
        />
        <FieldError>{valueError}</FieldError>
      </TextField>
      <TextField className="flex flex-col gap-1.5">
        <Label>Description (optional)</Label>
        <TextArea
          placeholder="When should the agent use this? e.g. Primary Postgres connection string for the analytics DB."
          value={description}
          disabled={pending}
          rows={2}
          fullWidth
          onChange={(event) => setDescription(event.target.value)}
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
        <Button type="submit" variant="primary" size="sm" isDisabled={pending}>
          {pending ? <Spinner color="current" size="sm" /> : null}
          Add
        </Button>
      </div>
    </form>
  )
}

export function EnvironmentVariableRow({
  deleting,
  deletePending,
  description,
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
  description: string
  editing: boolean
  name: string
  updateError: string | null
  updatePending: boolean
  onCancelDelete: () => void
  onCancelEdit: () => void
  onConfirmDelete: () => void
  onDelete: () => void
  onEdit: () => void
  onSubmitEdit: (newName: string, value: string, description: string) => void
}) {
  if (editing) {
    return (
      <EditVariableForm
        name={name}
        description={description}
        pending={updatePending}
        submitError={updateError}
        onCancel={onCancelEdit}
        onSubmit={onSubmitEdit}
      />
    )
  }

  return (
    <div className="flex items-start gap-3 rounded-xl border border-border bg-surface px-4 py-3.5">
      <div className="min-w-0 flex-1">
        <p className="truncate font-mono text-sm font-medium">{name}</p>
        {description ? (
          <p className="text-muted-foreground mt-1 text-xs">{description}</p>
        ) : null}
      </div>
      <EnvVarActionsMenu name={name} onEdit={onEdit} onDelete={onDelete} />

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
                    <AlertDialog.Heading>Delete “{name}”</AlertDialog.Heading>
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

function EnvVarActionsMenu({
  name,
  onEdit,
  onDelete,
}: {
  name: string
  onEdit: () => void
  onDelete: () => void
}) {
  const [open, setOpen] = useState(false)
  return (
    <Popover isOpen={open} onOpenChange={setOpen}>
      <Popover.Trigger
        aria-label={`${name} options`}
        data-open={open ? "true" : undefined}
        className="hover:bg-default data-[open=true]:bg-default -mr-1 flex shrink-0 items-center rounded-md p-1 text-muted-foreground transition-colors"
      >
        <AppIcon icon="ellipsis" className="h-4 w-4" />
      </Popover.Trigger>
      {open ? (
        <Popover.Content
          placement="bottom end"
          offset={6}
          className="w-44 rounded-2xl border border-border p-1.5"
        >
          <Popover.Dialog className="flex w-full flex-col gap-0.5 p-0">
            <button
              type="button"
              onClick={() => {
                onEdit()
                setOpen(false)
              }}
              className="hover:bg-default flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors"
            >
              <AppIcon icon="pencil" className="h-4 w-4 shrink-0" />
              Edit
            </button>
            <button
              type="button"
              onClick={() => {
                onDelete()
                setOpen(false)
              }}
              className="flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-sm text-danger transition-colors hover:bg-danger/10"
            >
              <AppIcon icon="trash-2" className="h-4 w-4 shrink-0" />
              Delete
            </button>
          </Popover.Dialog>
        </Popover.Content>
      ) : null}
    </Popover>
  )
}

function EditVariableForm({
  name,
  description: initialDescription,
  onCancel,
  onSubmit,
  pending,
  submitError,
}: {
  name: string
  description: string
  onCancel: () => void
  onSubmit: (newName: string, value: string, description: string) => void
  pending: boolean
  submitError: string | null
}) {
  const [nameDraft, setNameDraft] = useState(name)
  const [value, setValue] = useState("")
  const [description, setDescription] = useState(initialDescription)
  const [touched, setTouched] = useState(false)

  const normalizedName = normalizeEnvName(nameDraft)
  const nameError = validateEnvName(normalizedName)
  const unchanged =
    normalizedName === name &&
    value.length === 0 &&
    description.trim() === initialDescription

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setTouched(true)
    if (nameError || unchanged || pending) return
    onSubmit(normalizedName, value, description.trim())
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
        <SecretValueInput
          placeholder="Leave blank to keep the current value"
          value={value}
          disabled={pending}
          onChange={setValue}
        />
      </TextField>
      <TextField className="flex flex-col gap-1.5">
        <Label>Description (optional)</Label>
        <TextArea
          placeholder="When should the agent use this?"
          value={description}
          disabled={pending}
          rows={2}
          fullWidth
          onChange={(event) => setDescription(event.target.value)}
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
