"use client"

import { useCallback, useMemo, useState } from "react"
import Link from "next/link"
import { useParams, useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Input, Spinner, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { useIsAdmin } from "@/lib/auth/use-role"
import { ProviderIcon } from "../../_provider-icon"
import {
  deriveProvider,
  providerMeta,
  RAG_SOURCES_QUERY_KEY,
  type Connection,
} from "../../_lib"
import {
  Field,
  IntegrationScope,
  MultiSelect,
  WebsiteScope,
  type Option,
  type ScopeItem,
  type UrlOption,
} from "../../_form"
import { teamGrantDiff } from "../../_team-grants"
import { TeamGrantProbe } from "./_team-grant-probe"

const EMPTY_CONNECTIONS: Connection[] = []

export default function EditKnowledgeSourcePage() {
  const { id } = useParams<{ id: string }>()
  const router = useRouter()
  const queryClient = useQueryClient()
  // Editing a knowledge source (PATCH /v1/rag/sources/{id}) and
  // granting/revoking team access (POST/DELETE .../teams/{id}/rag-sources)
  // are admin-only on the backend. Non-admins see a read-only blocked state
  // instead of the edit form; the backend enforces this too.
  const isAdmin = useIsAdmin()

  const sourceQuery = $api.useQuery("get", "/v1/rag/sources/{id}", { params: { path: { id } } })
  const connectionsQuery = $api.useQuery("get", "/v1/connections", {
    params: { query: { limit: 100 } },
  })
  const teamsQuery = $api.useQuery("get", "/v1/orgs/current/teams", {
    params: { query: { limit: 100 } },
  })

  const source = sourceQuery.data
  const connections = connectionsQuery.data?.data ?? EMPTY_CONNECTIONS
  const connectionsById = useMemo(() => {
    const map = new Map<string, Connection>()
    for (const c of connections) if (c.id) map.set(c.id, c)
    return map
  }, [connections])

  const provider = source ? deriveProvider(source, connectionsById) : ""
  const meta = provider ? providerMeta(provider) : null
  const connectionId = source?.connection_id ?? undefined

  const teamOptions: Option[] = useMemo(
    () =>
      (teamsQuery.data?.data ?? []).map((t) => ({
        id: t.id ?? "",
        name: t.name ?? "",
      })),
    [teamsQuery.data]
  )

  const scopesQuery = $api.useQuery(
    "get",
    "/v1/rag/connections/{connection_id}/scopes",
    { params: { path: { connection_id: connectionId ?? "" } } },
    { enabled: Boolean(connectionId) && meta?.kind === "INTEGRATION" }
  )
  const scopeTypes = scopesQuery.data?.scopes ?? []

  const [name, setName] = useState("")
  const [scopeItems, setScopeItems] = useState<ScopeItem[]>([])
  const [websiteURLs, setWebsiteURLs] = useState<UrlOption[]>([])
  const [teams, setTeams] = useState<string[]>([])
  const [initialGrantedTeamIds, setInitialGrantedTeamIds] = useState<
    string[]
  >([])
  const [seededSource, setSeededSource] = useState(false)
  const [seededTeams, setSeededTeams] = useState(false)
  const [grantedByTeam, setGrantedByTeam] = useState<Record<string, boolean>>(
    {}
  )
  const handleProbeResult = useCallback((teamId: string, granted: boolean) => {
    setGrantedByTeam((prev) =>
      prev[teamId] === granted ? prev : { ...prev, [teamId]: granted }
    )
  }, [])

  // Seed name/scope from the source and team grants from their own probes
  // independently, so slow or empty probe responses can't block the rest.
  if (!seededSource && source) {
    const config = (source.config ?? {}) as {
      scope?: { resource_type?: string; items?: { id?: string; name?: string; type?: string }[] }
      urls?: string[]
    }
    // Older single-type sources store the type once at the scope level rather
    // than on each item; fall back to it so those items pre-select too.
    const fallbackType = config.scope?.resource_type ?? ""
    setName(source.name ?? "")
    setScopeItems(
      (config.scope?.items ?? []).map((i) => ({
        id: i.id ?? "",
        name: i.name ?? "",
        type: i.type || fallbackType,
      }))
    )
    setWebsiteURLs((config.urls ?? []).map((u) => ({ id: u, name: u, urls: [u] })))
    setSeededSource(true)
  }
  // Seed the team selection once every team's probe has reported. Each
  // probe only calls onResult after its own query resolves, so this
  // condition naturally waits out slow probes rather than racing them.
  if (
    !seededTeams &&
    teamOptions.length > 0 &&
    teamOptions.every((t) => t.id in grantedByTeam)
  ) {
    const granted = teamOptions
      .filter((t) => grantedByTeam[t.id])
      .map((t) => t.id)
    setTeams(granted)
    setInitialGrantedTeamIds(granted)
    setSeededTeams(true)
  }

  const updateSource = $api.useMutation("patch", "/v1/rag/sources/{id}")
  const grantSource = $api.useMutation(
    "post",
    "/v1/orgs/current/teams/{teamID}/rag-sources"
  )
  const revokeSource = $api.useMutation(
    "delete",
    "/v1/orgs/current/teams/{teamID}/rag-sources/{sourceID}"
  )
  const saving =
    updateSource.isPending || grantSource.isPending || revokeSource.isPending

  const scopeReady =
    meta?.kind === "WEBSITE"
      ? websiteURLs.length > 0
      : scopeTypes.length === 0 || scopeItems.length > 0
  const canSubmit =
    seededSource && name.trim() !== "" && scopeReady && teams.length > 0 && !saving

  async function save() {
    if (!meta || !canSubmit) return
    const config: Record<string, unknown> =
      meta.kind === "WEBSITE"
        ? { urls: Array.from(new Set(websiteURLs.flatMap((u) => u.urls))) }
        : scopeItems.length > 0
          ? {
              scope: {
                resource_type: scopeTypes.length === 1 ? (scopeTypes[0].key ?? "") : "",
                items: scopeItems.map((i) => ({ id: i.id, name: i.name, type: i.type })),
              },
            }
          : {}

    try {
      await updateSource.mutateAsync({
        params: { path: { id } },
        body: { name: name.trim(), config } as never,
      })
      const { grant, revoke } = teamGrantDiff(initialGrantedTeamIds, teams)
      for (const teamID of grant) {
        await grantSource.mutateAsync({
          params: { path: { teamID } },
          body: { rag_source_id: id },
        })
      }
      for (const teamID of revoke) {
        await revokeSource.mutateAsync({
          params: { path: { teamID, sourceID: id } },
        })
      }
      queryClient.invalidateQueries({ queryKey: RAG_SOURCES_QUERY_KEY })
      toast.success(`${name.trim()} updated`)
      router.push("/w/settings/knowledge")
    } catch (error) {
      toast.danger(extractErrorMessage(error, "Could not update source"))
    }
  }

  if (!isAdmin) {
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
            <h1 className="text-lg font-semibold text-foreground">
              Edit knowledge source
            </h1>
            <p className="mt-1 max-w-lg text-sm text-muted-foreground">
              Only workspace admins can edit knowledge sources.
            </p>
          </div>
        </div>
      </div>
    )
  }

  if (sourceQuery.isLoading || !source || !meta) {
    return (
      <div className="flex min-h-56 items-center justify-center">
        <Spinner size="sm" />
      </div>
    )
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
          <h1 className="text-lg font-semibold text-foreground">Edit knowledge source</h1>
          <p className="mt-1 max-w-lg text-sm text-muted-foreground">
            Update the name, what it ingests, and which teams can search it.
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

        <Field label="Source" hint="The source type can’t be changed.">
          <div className="flex items-center gap-3 rounded-xl border border-border bg-surface px-4 py-3">
            <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-default">
              <ProviderIcon icon={meta.icon} className="h-5 w-5" />
            </div>
            <span className="text-sm font-medium text-foreground">{meta.label}</span>
          </div>
        </Field>

        {meta.kind === "INTEGRATION" ? (
          <IntegrationScope
            meta={meta}
            connectionId={connectionId}
            scopesLoading={scopesQuery.isLoading}
            scopeTypes={scopeTypes.map((s) => ({ id: s.key ?? "", name: s.display_name ?? s.key ?? "" }))}
            items={scopeItems}
            onItems={setScopeItems}
          />
        ) : null}

        {meta.kind === "WEBSITE" ? (
          <WebsiteScope value={websiteURLs} onChange={setWebsiteURLs} />
        ) : null}

        {!seededTeams
          ? teamOptions.map((t) => (
              <TeamGrantProbe
                key={t.id}
                teamId={t.id}
                sourceId={id}
                onResult={handleProbeResult}
              />
            ))
          : null}

        <Field label="Teams" hint="The teams whose agents can search this source.">
          <MultiSelect
            ariaLabel="Teams"
            placeholder={
              teamsQuery.isLoading || !seededTeams
                ? "Loading teams…"
                : "Select teams"
            }
            options={teamOptions}
            value={teams}
            onChange={setTeams}
          />
        </Field>
      </div>

      <div className="flex items-center justify-end gap-2 pt-2">
        <Button
          type="button"
          variant="tertiary"
          size="sm"
          isDisabled={saving}
          onPress={() => router.push("/w/settings/knowledge")}
        >
          Cancel
        </Button>
        <Button type="button" variant="primary" size="sm" isDisabled={!canSubmit} onPress={save}>
          {saving ? <Spinner color="current" size="sm" /> : null}
          Save changes
        </Button>
      </div>
    </div>
  )
}
