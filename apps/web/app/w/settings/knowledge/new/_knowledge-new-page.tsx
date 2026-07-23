"use client"

import { useMemo, useState } from "react"
import Link from "next/link"
import { useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Input, Spinner, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { useIsAdmin } from "@/lib/auth/use-role"
import {
  connectionForProvider,
  PROVIDERS,
  providerMeta,
  RAG_SOURCES_QUERY_KEY,
  type Connection,
  type ProviderMeta,
} from "../_lib"
import {
  Field,
  IntegrationScope,
  MultiSelect,
  SourceCards,
  WebsiteScope,
  type Option,
  type ScopeItem,
  type UrlOption,
} from "../_form"

const EMPTY_CONNECTIONS: Connection[] = []

export default function KnowledgeNewPageContent() {
  const router = useRouter()
  const queryClient = useQueryClient()
  // Creating a knowledge source (POST /v1/rag/sources) and granting it to
  // teams (POST .../teams/{id}/rag-sources) are admin-only on the backend.
  // Non-admins never see the create form; the backend enforces this too.
  const isAdmin = useIsAdmin()

  const connectionsQuery = $api.useQuery("get", "/v1/connections", {
    params: { query: { limit: 100 } },
  })
  const teamsQuery = $api.useQuery("get", "/v1/orgs/current/teams", {
    params: { query: { limit: 100 } },
  })

  const connections = connectionsQuery.data?.data ?? EMPTY_CONNECTIONS
  const connectedProviders = useMemo(() => {
    const set = new Set<string>()
    for (const p of PROVIDERS) {
      if (p.kind === "WEBSITE" || connectionForProvider(p, connections)) {
        set.add(p.provider)
      }
    }
    return set
  }, [connections])

  const teamOptions: Option[] = useMemo(
    () =>
      (teamsQuery.data?.data ?? []).map((t) => ({
        id: t.id ?? "",
        name: t.name ?? "",
      })),
    [teamsQuery.data]
  )

  const [name, setName] = useState("")
  const [provider, setProvider] = useState<string>("")
  const [scopeItems, setScopeItems] = useState<ScopeItem[]>([])
  const [websiteURLs, setWebsiteURLs] = useState<UrlOption[]>([])
  const [teams, setTeams] = useState<string[]>([])

  const meta = provider ? providerMeta(provider) : null
  const connectionId = meta ? connectionForProvider(meta, connections)?.id : undefined

  const scopesQuery = $api.useQuery(
    "get",
    "/v1/rag/connections/{connection_id}/scopes",
    { params: { path: { connection_id: connectionId ?? "" } } },
    { enabled: Boolean(connectionId) && meta?.kind === "INTEGRATION" }
  )
  const scopeTypes = scopesQuery.data?.scopes ?? []

  const createSource = $api.useMutation("post", "/v1/rag/sources")
  const grantSource = $api.useMutation(
    "post",
    "/v1/orgs/current/teams/{teamID}/rag-sources"
  )
  const saving = createSource.isPending || grantSource.isPending

  const scopeReady =
    meta?.kind === "WEBSITE"
      ? websiteURLs.length > 0
      : scopeTypes.length === 0 || scopeItems.length > 0 // no scopes = ingest everything
  const canSubmit =
    name.trim() !== "" && provider !== "" && scopeReady && teams.length > 0 && !saving

  function selectProvider(next: ProviderMeta) {
    setProvider(next.provider)
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
      body.config = { urls: Array.from(new Set(websiteURLs.flatMap((u) => u.urls))) }
    } else {
      body.connection_id = connectionId
      const resourceType = scopeTypes.length === 1 ? (scopeTypes[0].key ?? "") : ""
      body.config =
        scopeItems.length > 0
          ? {
              scope: {
                resource_type: resourceType,
                items: scopeItems.map((i) => ({ id: i.id, name: i.name, type: i.type })),
              },
            }
          : {}
    }

    try {
      const created = await createSource.mutateAsync({ body: body as never })
      const id = created.id
      if (id) {
        for (const teamID of teams) {
          await grantSource.mutateAsync({
            params: { path: { teamID } },
            body: { rag_source_id: id },
          })
        }
      }
      queryClient.invalidateQueries({ queryKey: RAG_SOURCES_QUERY_KEY })
      toast.success(`${name.trim()} added`)
      router.push("/w/knowledge")
    } catch (error) {
      toast.danger(extractErrorMessage(error, "Could not add source"))
    }
  }

  if (!isAdmin) {
    return (
      <div className="flex flex-col gap-8">
        <div className="flex flex-col gap-3">
          <Link
            href="/w/knowledge"
            className="inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
          >
            <AppIcon icon="arrow-left" className="h-4 w-4" />
            Knowledge
          </Link>
          <div>
            <h1 className="text-lg font-semibold text-foreground">
              Add knowledge source
            </h1>
            <p className="mt-1 max-w-lg text-sm text-muted-foreground">
              Only workspace admins can create knowledge sources.
            </p>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-3">
        <Link
          href="/w/knowledge"
          className="inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
        >
          <AppIcon icon="arrow-left" className="h-4 w-4" />
          Knowledge
        </Link>
        <div>
          <h1 className="text-lg font-semibold text-foreground">Add knowledge source</h1>
          <p className="mt-1 max-w-lg text-sm text-muted-foreground">
            Give the source a name, pick where it pulls from, scope it to specific
            resources, and choose which teams can search it.
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
            connectedProviders={connectedProviders}
          />
        </Field>

        {meta?.kind === "INTEGRATION" ? (
          <IntegrationScope
            meta={meta}
            connectionId={connectionId}
            scopesLoading={scopesQuery.isLoading}
            scopeTypes={scopeTypes.map((s) => ({ id: s.key ?? "", name: s.display_name ?? s.key ?? "" }))}
            items={scopeItems}
            onItems={setScopeItems}
          />
        ) : null}

        {meta?.kind === "WEBSITE" ? (
          <WebsiteScope value={websiteURLs} onChange={setWebsiteURLs} />
        ) : null}

        <Field label="Teams" hint="The teams whose agents can search this source.">
          <MultiSelect
            ariaLabel="Teams"
            placeholder={teamsQuery.isLoading ? "Loading teams…" : "Select teams"}
            options={teamOptions}
            value={teams}
            onChange={setTeams}
          />
        </Field>
      </div>

      <div className="flex items-center justify-end gap-2 pt-2">
        <Button type="button" variant="tertiary" size="sm" isDisabled={saving} onPress={() => router.push("/w/knowledge")}>
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
