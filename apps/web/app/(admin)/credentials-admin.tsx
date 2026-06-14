"use client"

import { useMemo, useState, type FormEvent } from "react"
import { Button, Label, Popover } from "@heroui/react"
import { Icon } from "@iconify/react"
import { Field } from "./admin-field"
import {
  type CredentialForm,
  type LLMProvider,
  type SystemCredential,
} from "./types"

export function CredentialPanel({
  credentials,
  providers,
  form,
  saving,
  revokingID,
  onFormChange,
  onSubmit,
  onRevoke,
}: {
  credentials: SystemCredential[]
  providers: LLMProvider[]
  form: CredentialForm
  saving: boolean
  revokingID: string | null
  onFormChange: (value: CredentialForm) => void
  onSubmit: (event: FormEvent<HTMLFormElement>) => void
  onRevoke: (id: string) => void
}) {
  function updateForm(patch: Partial<CredentialForm>) {
    onFormChange({ ...form, ...patch })
  }

  function selectProvider(providerID: string) {
    const provider = providers.find((item) => item.id === providerID)
    updateForm({
      provider_id: providerID,
      label: form.label || provider?.name || providerID,
      base_url: form.base_url || provider?.base_url || "",
      auth_scheme:
        form.auth_scheme || provider?.default_auth_scheme || "bearer",
    })
  }

  return (
    <div className="grid gap-5 lg:grid-cols-[minmax(0,1fr)_360px]">
      <div className="grid content-start gap-3">
        {credentials.length > 0 ? (
          credentials.map((credential) => (
            <CredentialRow
              key={credential.id}
              credential={credential}
              revoking={revokingID === credential.id}
              onRevoke={() => onRevoke(credential.id)}
            />
          ))
        ) : (
          <div className="bg-surface rounded-2xl border border-border px-6 py-10 text-center text-sm text-muted">
            No system credentials found.
          </div>
        )}
      </div>

      <form
        onSubmit={onSubmit}
        className="bg-surface grid content-start gap-4 rounded-2xl border border-border p-4"
      >
        <div>
          <h2 className="text-sm font-medium">Add system credential</h2>
          <p className="mt-1 text-xs text-muted">
            Stored encrypted and used as a global fallback credential.
          </p>
        </div>
        <ProviderPicker
          providers={providers}
          value={form.provider_id}
          onChange={selectProvider}
        />
        <Field
          id="credential-label"
          label="Label"
          value={form.label}
          onChange={(value) => updateForm({ label: value })}
          placeholder="OpenAI production"
        />
        <Field
          id="credential-base-url"
          label="Base URL"
          value={form.base_url}
          onChange={(value) => updateForm({ base_url: value })}
          placeholder="https://api.openai.com/v1"
          required
        />
        <Field
          id="credential-auth-scheme"
          label="Auth scheme"
          value={form.auth_scheme}
          onChange={(value) => updateForm({ auth_scheme: value })}
          placeholder="bearer"
          required
        />
        <Field
          id="credential-api-key"
          label="Credential"
          type="password"
          value={form.api_key}
          onChange={(value) => updateForm({ api_key: value })}
          required
        />
        <Button
          type="submit"
          variant="primary"
          isPending={saving}
          isDisabled={!form.provider_id}
        >
          Save credential
        </Button>
      </form>
    </div>
  )
}

function ProviderPicker({
  providers,
  value,
  onChange,
}: {
  providers: LLMProvider[]
  value: string
  onChange: (providerID: string) => void
}) {
  const [open, setOpen] = useState(false)
  const selected = useMemo(
    () => providers.find((provider) => provider.id === value),
    [providers, value]
  )

  return (
    <div className="grid gap-2">
      <Label>Provider</Label>
      <Popover isOpen={open} onOpenChange={setOpen}>
        <Popover.Trigger
          aria-label="Select provider"
          className="bg-field-background hover:bg-default flex h-11 w-full items-center justify-between rounded-2xl border border-border px-3 text-sm text-foreground transition-colors"
        >
          <span className={selected ? "text-foreground" : "text-muted"}>
            {selected?.name || selected?.id || "Select a provider"}
          </span>
          <Icon icon="lucide:chevron-down" className="size-4 text-muted" />
        </Popover.Trigger>
        <Popover.Content className="w-72 rounded-2xl border border-border bg-background p-1.5 shadow-xl">
          <Popover.Dialog className="flex max-h-72 w-full flex-col gap-0.5 overflow-y-auto p-0">
            {providers.map((provider) => (
              <button
                key={provider.id}
                type="button"
                onClick={() => {
                  onChange(provider.id)
                  setOpen(false)
                }}
                className="hover:bg-default flex items-center gap-2 rounded-xl px-3 py-2 text-left text-sm transition-colors"
              >
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-medium">
                    {provider.name || provider.id}
                  </span>
                  <span className="block truncate text-xs text-muted">
                    {provider.base_url || provider.default_auth_scheme}
                  </span>
                </span>
                {provider.id === value ? (
                  <Icon icon="lucide:check" className="size-4 shrink-0" />
                ) : null}
              </button>
            ))}
          </Popover.Dialog>
        </Popover.Content>
      </Popover>
    </div>
  )
}

function CredentialRow({
  credential,
  revoking,
  onRevoke,
}: {
  credential: SystemCredential
  revoking: boolean
  onRevoke: () => void
}) {
  const revoked = Boolean(credential.revoked_at)

  return (
    <div className="bg-surface flex items-center justify-between gap-4 rounded-2xl border border-border px-4 py-3">
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <p className="truncate text-sm font-medium">
            {credential.label || credential.provider_id || "System credential"}
          </p>
          <span
            className={
              revoked
                ? "rounded-full border border-border px-2 py-0.5 text-[11px] font-medium text-muted"
                : "bg-default rounded-full border border-border px-2 py-0.5 text-[11px] font-medium text-foreground"
            }
          >
            {revoked ? "Revoked" : "Active"}
          </span>
        </div>
        <p className="mt-1 truncate text-xs text-muted">
          {credential.provider_id ?? "unknown"} · {credential.base_url}
        </p>
      </div>
      <Button
        type="button"
        size="sm"
        variant="secondary"
        isDisabled={revoked}
        isPending={revoking}
        onPress={onRevoke}
      >
        Revoke
      </Button>
    </div>
  )
}
