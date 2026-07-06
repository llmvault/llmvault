"use client"

import { useMemo, useState, type ComponentProps } from "react"
import Link from "next/link"
import { useRouter } from "next/navigation"
import { Button, Input, ListBox, Popover, Select, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { cn } from "@/lib/utils"
import { CHANNELS, PROVIDERS, providerMeta, type Provider } from "../_data"

export default function NewKnowledgeSourcePage() {
  const router = useRouter()

  const [name, setName] = useState("")
  const [provider, setProvider] = useState<Provider | "">("")
  const [resourceIds, setResourceIds] = useState<string[]>([])
  const [channel, setChannel] = useState("")

  const meta = provider ? providerMeta(provider) : null
  const canSubmit =
    name.trim() !== "" &&
    provider !== "" &&
    resourceIds.length > 0 &&
    channel !== ""

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
            specific resources, and choose which channel can search it.
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
            <ResourceMultiSelect
              meta={meta}
              value={resourceIds}
              onChange={setResourceIds}
            />
          </Field>
        ) : null}

        <Field
          label="Channel"
          hint="The channel whose agents can search this source."
        >
          <ChannelSelect value={channel} onChange={setChannel} />
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
              <AppIcon icon={p.icon} className="h-5 w-5" />
            </div>
            <div className="flex min-w-0 flex-1 flex-col gap-0.5">
              <span className="text-sm font-medium text-foreground">
                {p.label}
              </span>
              <span className="truncate text-xs text-muted-foreground">
                Scoped by {p.resourceLabel.toLowerCase()}
              </span>
            </div>
            <span
              className={cn(
                "flex h-4 w-4 shrink-0 items-center justify-center rounded-full border transition-colors",
                selected ? "border-primary bg-primary text-primary-foreground" : "border-border"
              )}
            >
              {selected ? <AppIcon icon="check" className="h-3 w-3" /> : null}
            </span>
          </button>
        )
      })}
    </div>
  )
}

function ChannelSelect({
  value,
  onChange,
}: {
  value: string
  onChange: (value: string) => void
}) {
  const selected = CHANNELS.find((c) => c.id === value)
  return (
    <Select
      aria-label="Channel"
      value={value}
      onChange={(key) => onChange(String(key))}
      className="w-full"
    >
      <Select.Trigger className="h-10 w-full justify-between px-3 text-sm">
        <span className="flex items-center gap-2">
          {selected ? (
            <>
              <AppIcon icon="hash" className="h-4 w-4 text-muted-foreground" />
              {selected.name}
            </>
          ) : (
            <span className="text-muted-foreground">Select a channel</span>
          )}
        </span>
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover className="w-[var(--trigger-width)] p-1.5">
        <ListBox>
          {CHANNELS.map((c) => (
            <ListBox.Item key={c.id} id={c.id} textValue={c.name}>
              <span className="flex items-center gap-2">
                <AppIcon icon="hash" className="h-4 w-4 text-muted-foreground" />
                {c.name}
              </span>
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}

function ResourceMultiSelect({
  meta,
  value,
  onChange,
  placement = "bottom start",
}: {
  meta: ReturnType<typeof providerMeta>
  value: string[]
  onChange: (value: string[]) => void
  placement?: ComponentProps<typeof Popover.Content>["placement"]
}) {
  const [open, setOpen] = useState(false)
  const summary = useMemo(() => {
    if (value.length === 0) return `Select ${meta.resourceLabel.toLowerCase()}`
    if (value.length === 1) {
      return meta.resources.find((r) => r.id === value[0])?.name ?? "1 selected"
    }
    return `${value.length} selected`
  }, [value, meta])

  function toggle(id: string) {
    onChange(
      value.includes(id) ? value.filter((v) => v !== id) : [...value, id]
    )
  }

  return (
    <Popover isOpen={open} onOpenChange={setOpen}>
      <Popover.Trigger
        data-open={open ? "true" : undefined}
        className="flex h-10 w-full items-center justify-between rounded-md border border-border bg-card px-3 text-sm transition-colors hover:border-muted-foreground/40"
      >
        <span className={cn(value.length === 0 && "text-muted-foreground")}>
          {summary}
        </span>
        <AppIcon icon="chevron-down" className="h-4 w-4 text-muted-foreground" />
      </Popover.Trigger>
      {open ? (
        <Popover.Content
          placement={placement}
          offset={6}
          className="w-[var(--trigger-width)] rounded-2xl border border-border p-1.5"
        >
          <Popover.Dialog className="flex max-h-72 w-full flex-col gap-0.5 overflow-y-auto p-0">
            {meta.resources.map((resource) => {
              const checked = value.includes(resource.id)
              return (
                <button
                  key={resource.id}
                  type="button"
                  onClick={() => toggle(resource.id)}
                  className="hover:bg-default flex items-center gap-2.5 rounded-xl px-2.5 py-1.5 text-left text-sm transition-colors"
                >
                  <span
                    className={cn(
                      "flex h-4 w-4 shrink-0 items-center justify-center rounded border transition-colors",
                      checked
                        ? "border-primary bg-primary text-primary-foreground"
                        : "border-border"
                    )}
                  >
                    {checked ? (
                      <AppIcon icon="check" className="h-3 w-3" />
                    ) : null}
                  </span>
                  <span className="min-w-0 flex-1 truncate">{resource.name}</span>
                </button>
              )
            })}
          </Popover.Dialog>
        </Popover.Content>
      ) : null}
    </Popover>
  )
}
