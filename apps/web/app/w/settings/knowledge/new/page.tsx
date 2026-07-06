"use client"

import { useMemo, useState } from "react"
import Link from "next/link"
import { useRouter } from "next/navigation"
import { Button, Input, ListBox, Popover, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { cn } from "@/lib/utils"
import { ProviderIcon } from "../_provider-icon"
import { CHANNELS, PROVIDERS, providerMeta, type Provider } from "../_data"

export default function NewKnowledgeSourcePage() {
  const router = useRouter()

  const [name, setName] = useState("")
  const [provider, setProvider] = useState<Provider | "">("")
  const [resourceIds, setResourceIds] = useState<string[]>([])
  const [channels, setChannels] = useState<string[]>([])

  const meta = provider ? providerMeta(provider) : null
  const canSubmit =
    name.trim() !== "" &&
    provider !== "" &&
    resourceIds.length > 0 &&
    channels.length > 0

  function selectProvider(next: Provider) {
    setProvider(next)
    setResourceIds([]) // reset the scope when the source changes
  }

  function save() {
    if (!canSubmit) return
    // Static for now — no API call. Persisting happens once wired to
    // POST /v1/rag/sources with the scope envelope + channel grant.
    toast.success(`${name.trim()} added`)
    router.push("/w/settings/knowledge")
  }

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-3">
        <Link
          href="/w/settings/knowledge"
          className="inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
        >
          <AppIcon icon="arrow-left" className="h-4 w-4" />
          Knowledge
        </Link>
        <div>
          <h1 className="text-2xl font-semibold text-foreground">
            Add knowledge source
          </h1>
          <p className="mt-1 max-w-lg text-sm text-muted-foreground">
            Give the source a name, pick where it pulls from, scope it to
            specific resources, and choose which channels can search it.
          </p>
        </div>
      </div>

      <div className="flex flex-col gap-6">
        <Field label="Name" hint="A short label to recognize this source.">
          <Input
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder="e.g. Engineering repos"
            className="h-10 w-full rounded-md bg-card"
          />
        </Field>

        <Field label="Source" hint="Where this knowledge comes from.">
          <SourceCards value={provider} onChange={selectProvider} />
        </Field>

        {meta ? (
          <Field label={meta.resourceLabel} hint={meta.resourceHint}>
            <MultiSelect
              ariaLabel={meta.resourceLabel}
              placeholder={`Select ${meta.resourceLabel.toLowerCase()}`}
              options={meta.resources}
              value={resourceIds}
              onChange={setResourceIds}
            />
          </Field>
        ) : null}

        <Field
          label="Channels"
          hint="The channels whose agents can search this source."
        >
          <MultiSelect
            ariaLabel="Channels"
            placeholder="Select channels"
            options={CHANNELS}
            value={channels}
            onChange={setChannels}
          />
        </Field>
      </div>

      <div className="flex items-center justify-end gap-2 pt-2">
        <Button
          type="button"
          variant="tertiary"
          size="sm"
          onPress={() => router.push("/w/settings/knowledge")}
        >
          Cancel
        </Button>
        <Button
          type="button"
          variant="primary"
          size="sm"
          isDisabled={!canSubmit}
          onPress={save}
        >
          Add source
        </Button>
      </div>
    </div>
  )
}

function Field({
  label,
  hint,
  children,
}: {
  label: string
  hint: string
  children: React.ReactNode
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex flex-col gap-0.5">
        <span className="text-sm font-medium text-foreground">{label}</span>
        <span className="text-xs text-muted-foreground">{hint}</span>
      </div>
      {children}
    </div>
  )
}

function SourceCards({
  value,
  onChange,
}: {
  value: Provider | ""
  onChange: (value: Provider) => void
}) {
  return (
    <div className="flex flex-col gap-2">
      {PROVIDERS.map((p) => {
        const selected = value === p.id
        return (
          <button
            key={p.id}
            type="button"
            onClick={() => onChange(p.id)}
            aria-pressed={selected}
            className={cn(
              "flex items-center gap-3 rounded-xl border bg-surface px-4 py-3 text-left transition-colors",
              selected
                ? "border-primary ring-1 ring-primary"
                : "border-border hover:border-muted-foreground/40"
            )}
          >
            <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-default">
              <ProviderIcon icon={p.icon} className="h-5 w-5" />
            </div>
            <div className="flex min-w-0 flex-1 flex-col gap-0.5">
              <span className="text-sm font-medium text-foreground">
                {p.label}
              </span>
              <span className="truncate text-xs text-muted-foreground">
                Scoped by {p.resourceLabel.toLowerCase()}
              </span>
            </div>
            {selected ? (
              <AppIcon
                icon="circle-check"
                className="h-5 w-5 shrink-0 text-success"
              />
            ) : null}
          </button>
        )
      })}
    </div>
  )
}

function MultiSelect({
  options,
  value,
  onChange,
  placeholder,
  ariaLabel,
}: {
  options: { id: string; name: string }[]
  value: string[]
  onChange: (value: string[]) => void
  placeholder: string
  ariaLabel: string
}) {
  const [open, setOpen] = useState(false)
  const summary = useMemo(() => {
    if (value.length === 0) return placeholder
    if (value.length === 1) {
      return options.find((o) => o.id === value[0])?.name ?? "1 selected"
    }
    return `${value.length} selected`
  }, [value, options, placeholder])

  return (
    <Popover isOpen={open} onOpenChange={setOpen}>
      {/* Reuses heroui's own select trigger/indicator/popover classes so it is
          visually identical to a native Select, while a real multiple ListBox
          drives selection. */}
      <Popover.Trigger
        aria-label={ariaLabel}
        data-open={open ? "true" : undefined}
        className="select__trigger select__trigger--full-width h-10 w-full justify-between px-3 text-sm transition-colors"
      >
        <span className={cn(value.length === 0 && "text-muted-foreground")}>
          {summary}
        </span>
        <svg
          width="16"
          height="16"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
          aria-hidden
          className={cn(
            "shrink-0 text-muted-foreground transition-transform",
            open && "rotate-180"
          )}
        >
          <path d="m6 9 6 6 6-6" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      </Popover.Trigger>
      <Popover.Content
        placement="bottom start"
        offset={6}
        className="select__popover w-[var(--trigger-width)] p-1.5"
      >
        <ListBox
          aria-label={ariaLabel}
          selectionMode="multiple"
          selectedKeys={new Set(value)}
          onSelectionChange={(keys) =>
            onChange(
              keys === "all"
                ? options.map((o) => o.id)
                : Array.from(keys, String)
            )
          }
        >
          {options.map((option) => (
            <ListBox.Item key={option.id} id={option.id} textValue={option.name}>
              {({ isSelected }) => (
                <span className="flex w-full items-center justify-between gap-2">
                  <span className="min-w-0 truncate">{option.name}</span>
                  {isSelected ? (
                    <AppIcon icon="tick-02" className="h-4 w-4 shrink-0" />
                  ) : null}
                </span>
              )}
            </ListBox.Item>
          ))}
        </ListBox>
      </Popover.Content>
    </Popover>
  )
}
