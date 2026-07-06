"use client"

import { useMemo, useState } from "react"
import Link from "next/link"
import { useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Input, ListBox, Popover, Spinner, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { cn } from "@/lib/utils"
import { ProviderIcon } from "../_provider-icon"
import {
  PROVIDERS,
  providerMeta,
  RAG_SOURCES_QUERY_KEY,
  type Connection,
  type ProviderMeta,
} from "../_lib"

type Option = { id: string; name: string }

const EMPTY_CONNECTIONS: Connection[] = []

export default function NewKnowledgeSourcePage() {
  const router = useRouter()
  const queryClient = useQueryClient()

  const connectionsQuery = $api.useQuery("get", "/v1/connections", {
    params: { query: { limit: 100 } },
  })
  const channelsQuery = $api.useQuery("get", "/v1/channels", {
    params: { query: { limit: 100 } },
  })

  const connections = connectionsQuery.data?.data ?? EMPTY_CONNECTIONS
  const connectionByProvider = useMemo(() => {
    const map = new Map<string, Connection>()
    for (const conn of connections) {
      const provider = conn.provider ?? ""
      if (!provider || conn.revoked_at || map.has(provider)) continue
      map.set(provider, conn)
    }
    return map
  }, [connections])

  const channelOptions: Option[] = useMemo(
    () =>
      (channelsQuery.data?.data ?? [])
        .filter((c) => !c.external_provider && !c.external_connection_id)
        .map((c) => ({ id: c.id ?? "", name: c.name ?? "" })),
    [channelsQuery.data]
  )

  const [name, setName] = useState("")
  const [provider, setProvider] = useState<string>("")
  // Integration scope: the chosen resource type + selected resource ids/names.
  const [scopeType, setScopeType] = useState<string>("")
  const [scopeItems, setScopeItems] = useState<Option[]>([])
  // Website scope: the selected seed URLs (sections + pages).
  const [websiteURLs, setWebsiteURLs] = useState<Option[]>([])
  const [channels, setChannels] = useState<string[]>([])

  const meta = provider ? providerMeta(provider) : null
  const connectionId = provider ? connectionByProvider.get(provider)?.id : undefined

  // Scopes available for the chosen integration connection.
  const scopesQuery = $api.useQuery(
    "get",
    "/v1/rag/connections/{connection_id}/scopes",
    { params: { path: { connection_id: connectionId ?? "" } } },
    { enabled: Boolean(connectionId) && meta?.kind === "INTEGRATION" }
  )
  const scopeTypes = scopesQuery.data?.scopes ?? []

  const createSource = $api.useMutation("post", "/v1/rag/sources")
  const setSourceChannels = $api.useMutation("put", "/v1/rag/sources/{id}/channels")
  const saving = createSource.isPending || setSourceChannels.isPending

  const scopeReady =
    meta?.kind === "WEBSITE"
      ? websiteURLs.length > 0
      : scopeTypes.length === 0 || scopeItems.length > 0 // no scopes = ingest everything
  const canSubmit =
    name.trim() !== "" && provider !== "" && scopeReady && channels.length > 0 && !saving

  function selectProvider(next: ProviderMeta) {
    setProvider(next.provider)
    setScopeType("")
    setScopeItems([])
    setWebsiteURLs([])
  }

  async function save() {
    if (!meta || !canSubmit) return
    const body: Record<string, unknown> = {
      name: name.trim(),
      kind: meta.kind,
    }
    if (meta.kind === "WEBSITE") {
      body.access_type = "PUBLIC"
      body.config = { urls: websiteURLs.map((u) => u.id) }
    } else {
      body.access_type = "PRIVATE"
      body.connection_id = connectionId
      body.config =
        scopeItems.length > 0
          ? { scope: { resource_type: scopeType, items: scopeItems.map((i) => ({ id: i.id, name: i.name })) } }
          : {}
    }

    try {
      const created = await createSource.mutateAsync({ body: body as never })
      const id = created.id
      if (id) {
        await setSourceChannels.mutateAsync({
          params: { path: { id } },
          body: { channel_ids: channels },
        })
      }
      queryClient.invalidateQueries({ queryKey: RAG_SOURCES_QUERY_KEY })
      toast.success(`${name.trim()} added`)
      router.push("/w/settings/knowledge")
    } catch (error) {
      toast.danger(extractErrorMessage(error, "Could not add source"))
    }
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
          <h1 className="text-2xl font-semibold text-foreground">Add knowledge source</h1>
          <p className="mt-1 max-w-lg text-sm text-muted-foreground">
            Give the source a name, pick where it pulls from, scope it to specific
            resources, and choose which channels can search it.
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
          <SourceCards
            value={provider}
            onChange={selectProvider}
            connectedProviders={connectionByProvider}
          />
        </Field>

        {meta?.kind === "INTEGRATION" ? (
          <IntegrationScope
            meta={meta}
            connectionId={connectionId}
            scopesLoading={scopesQuery.isLoading}
            scopeTypes={scopeTypes.map((s) => ({ id: s.key ?? "", name: s.display_name ?? s.key ?? "" }))}
            scopeType={scopeType}
            onScopeType={(t) => {
              setScopeType(t)
              setScopeItems([])
            }}
            items={scopeItems}
            onItems={setScopeItems}
          />
        ) : null}

        {meta?.kind === "WEBSITE" ? (
          <WebsiteScope value={websiteURLs} onChange={setWebsiteURLs} />
        ) : null}

        <Field label="Channels" hint="The channels whose agents can search this source.">
          <MultiSelect
            ariaLabel="Channels"
            placeholder={channelsQuery.isLoading ? "Loading channels…" : "Select channels"}
            options={channelOptions}
            value={channels}
            onChange={setChannels}
          />
        </Field>
      </div>

      <div className="flex items-center justify-end gap-2 pt-2">
        <Button type="button" variant="tertiary" size="sm" isDisabled={saving} onPress={() => router.push("/w/settings/knowledge")}>
          Cancel
        </Button>
        <Button type="button" variant="primary" size="sm" isDisabled={!canSubmit} onPress={save}>
          {saving ? <Spinner color="current" size="sm" /> : null}
          Add source
        </Button>
      </div>
    </div>
  )
}

function Field({ label, hint, children }: { label: string; hint: string; children: React.ReactNode }) {
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
  connectedProviders,
}: {
  value: string
  onChange: (value: ProviderMeta) => void
  connectedProviders: Map<string, Connection>
}) {
  return (
    <div className="flex flex-col gap-2">
      {PROVIDERS.map((p) => {
        const selected = value === p.provider
        const connected = p.kind === "WEBSITE" || connectedProviders.has(p.provider)
        return (
          <button
            key={p.provider}
            type="button"
            disabled={!connected}
            onClick={() => connected && onChange(p)}
            aria-pressed={selected}
            className={cn(
              "flex items-center gap-3 rounded-xl border bg-surface px-4 py-3 text-left transition-colors",
              !connected && "cursor-not-allowed opacity-55",
              selected
                ? "border-primary ring-1 ring-primary"
                : connected && "border-border hover:border-muted-foreground/40",
              !selected && !connected && "border-border"
            )}
          >
            <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-default">
              <ProviderIcon icon={p.icon} className="h-5 w-5" />
            </div>
            <div className="flex min-w-0 flex-1 flex-col gap-0.5">
              <span className="text-sm font-medium text-foreground">{p.label}</span>
              <span className="truncate text-xs text-muted-foreground">
                {connected ? `Scoped by ${p.scopeNoun}` : "Not connected"}
              </span>
            </div>
            {!connected ? (
              <Link
                href="/w/settings"
                onClick={(e) => e.stopPropagation()}
                className="shrink-0 text-xs font-medium text-primary transition-colors hover:text-primary/80"
              >
                Connect
              </Link>
            ) : selected ? (
              <AppIcon icon="circle-check" className="h-5 w-5 shrink-0 text-success" />
            ) : null}
          </button>
        )
      })}
    </div>
  )
}

function IntegrationScope({
  meta,
  connectionId,
  scopesLoading,
  scopeTypes,
  scopeType,
  onScopeType,
  items,
  onItems,
}: {
  meta: ProviderMeta
  connectionId?: string
  scopesLoading: boolean
  scopeTypes: Option[]
  scopeType: string
  onScopeType: (t: string) => void
  items: Option[]
  onItems: (items: Option[]) => void
}) {
  // Default to the first scope type once loaded.
  const effectiveType = scopeType || scopeTypes[0]?.id || ""

  // Fetch selectable resources for the chosen scope type.
  const resourcesQuery = $api.useQuery(
    "get",
    "/v1/connections/{id}/resources/{type}",
    { params: { path: { id: connectionId ?? "", type: effectiveType } } },
    { enabled: Boolean(connectionId) && Boolean(effectiveType) }
  )
  const options: Option[] = (resourcesQuery.data?.resources ?? []).map((r) => ({
    id: r.id ?? "",
    name: r.name ?? r.id ?? "",
  }))

  if (scopesLoading) {
    return (
      <Field label={meta.scopeNoun} hint="Loading available resources…">
        <div className="flex h-10 items-center px-1 text-sm text-muted-foreground">
          <Spinner size="sm" />
        </div>
      </Field>
    )
  }

  // No scopable types → this source ingests everything the connection can see.
  if (scopeTypes.length === 0) {
    return (
      <Field label="Scope" hint="This source will ingest everything this connection can access.">
        <div className="rounded-md border border-border bg-card px-3 py-2.5 text-sm text-muted-foreground">
          All {meta.label} content
        </div>
      </Field>
    )
  }

  const typeMeta = scopeTypes.find((t) => t.id === effectiveType)
  return (
    <Field
      label={typeMeta?.name ?? meta.scopeNoun}
      hint={`Choose the ${(typeMeta?.name ?? meta.scopeNoun).toLowerCase()} this source ingests.`}
    >
      <div className="flex flex-col gap-2">
        {scopeTypes.length > 1 ? (
          <div className="flex items-center gap-1">
            {scopeTypes.map((t) => (
              <button
                key={t.id}
                type="button"
                onClick={() => onScopeType(t.id)}
                className={cn(
                  "rounded-lg px-2.5 py-1 text-sm transition-colors",
                  t.id === effectiveType ? "bg-default font-medium" : "text-muted-foreground hover:text-foreground"
                )}
              >
                {t.name}
              </button>
            ))}
          </div>
        ) : null}
        <MultiSelect
          ariaLabel={typeMeta?.name ?? meta.scopeNoun}
          placeholder={resourcesQuery.isLoading ? "Loading…" : `Select ${(typeMeta?.name ?? meta.scopeNoun).toLowerCase()}`}
          options={options}
          value={items.map((i) => i.id)}
          onChange={(ids) => onItems(options.filter((o) => ids.includes(o.id)))}
        />
      </div>
    </Field>
  )
}

function WebsiteScope({
  value,
  onChange,
}: {
  value: Option[]
  onChange: (urls: Option[]) => void
}) {
  const [url, setUrl] = useState("")
  const discover = $api.useMutation("post", "/v1/rag/website/discover-sections")
  const result = discover.data

  const options: Option[] = useMemo(() => {
    if (!result) return []
    const sections = (result.sections ?? []).map((s) => ({
      id: s.url ?? "",
      name: `${s.label} · ${s.page_count} pages`,
    }))
    const pages = (result.pages ?? []).map((p) => ({
      id: p.url ?? "",
      name: p.path ?? p.url ?? "",
    }))
    return [...sections, ...pages]
  }, [result])

  async function runDiscover() {
    if (!url.trim()) return
    try {
      await discover.mutateAsync({ body: { url: url.trim() } })
      onChange([])
    } catch (error) {
      toast.danger(extractErrorMessage(error, "Couldn’t discover sections"))
    }
  }

  return (
    <Field label="Website" hint="Enter a site, discover its sections, then pick what to ingest.">
      <div className="flex flex-col gap-2">
        <div className="flex items-center gap-2">
          <Input
            value={url}
            onChange={(event) => setUrl(event.target.value)}
            placeholder="https://example.com"
            className="h-10 flex-1 rounded-md bg-card"
          />
          <Button
            type="button"
            variant="secondary"
            size="sm"
            isDisabled={!url.trim() || discover.isPending}
            onPress={runDiscover}
          >
            {discover.isPending ? <Spinner color="current" size="sm" /> : null}
            Discover
          </Button>
        </div>
        {result ? (
          options.length > 0 ? (
            <MultiSelect
              ariaLabel="Website sections"
              placeholder="Select sections & pages"
              options={options}
              value={value.map((v) => v.id)}
              onChange={(ids) => onChange(options.filter((o) => ids.includes(o.id)))}
            />
          ) : (
            <p className="text-xs text-muted-foreground">No sections found for this site.</p>
          )
        ) : null}
      </div>
    </Field>
  )
}

function MultiSelect({
  options,
  value,
  onChange,
  placeholder,
  ariaLabel,
}: {
  options: Option[]
  value: string[]
  onChange: (value: string[]) => void
  placeholder: string
  ariaLabel: string
}) {
  const [open, setOpen] = useState(false)
  const summary = useMemo(() => {
    if (value.length === 0) return placeholder
    if (value.length === 1) return options.find((o) => o.id === value[0])?.name ?? "1 selected"
    return `${value.length} selected`
  }, [value, options, placeholder])

  return (
    <Popover isOpen={open} onOpenChange={setOpen}>
      <Popover.Trigger
        aria-label={ariaLabel}
        data-open={open ? "true" : undefined}
        className="select__trigger select__trigger--full-width h-10 w-full justify-between px-3 text-sm transition-colors"
      >
        <span className={cn(value.length === 0 && "text-muted-foreground")}>{summary}</span>
        <svg
          width="16"
          height="16"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
          aria-hidden
          className={cn("shrink-0 text-muted-foreground transition-transform", open && "rotate-180")}
        >
          <path d="m6 9 6 6 6-6" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      </Popover.Trigger>
      <Popover.Content placement="bottom start" offset={6} className="select__popover w-[var(--trigger-width)] p-1.5">
        <ListBox
          aria-label={ariaLabel}
          selectionMode="multiple"
          selectedKeys={new Set(value)}
          onSelectionChange={(keys) =>
            onChange(keys === "all" ? options.map((o) => o.id) : Array.from(keys, String))
          }
        >
          {options.map((option) => (
            <ListBox.Item key={option.id} id={option.id} textValue={option.name}>
              {({ isSelected }) => (
                <span className="flex w-full items-center justify-between gap-2">
                  <span className="min-w-0 truncate">{option.name}</span>
                  {isSelected ? <AppIcon icon="tick-02" className="h-4 w-4 shrink-0" /> : null}
                </span>
              )}
            </ListBox.Item>
          ))}
        </ListBox>
      </Popover.Content>
    </Popover>
  )
}
