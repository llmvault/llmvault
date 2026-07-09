"use client"

import { useMemo } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "@heroui/react"
import { PluginLogoTile } from "@/components/plugin-logo"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { useIsAdmin } from "@/lib/auth/use-role"
import { pluginName, type ApiPlugin } from "@/app/w/(chat)/plugins/_lib"
import {
  deriveProvider,
  providerMeta,
  type Connection,
  type RagSource,
} from "../../knowledge/_lib"
import { ProviderIcon } from "../../knowledge/_provider-icon"
import {
  enabledIdSet,
  isProvisioned,
  isTeamProvisionable,
} from "./_provisioning-lib"
import {
  EmptyProvisioningRow,
  ProvisioningRow,
  ProvisioningSkeleton,
  SectionHeader,
} from "./_provisioning-row"

const TEAM_PLUGINS_KEY = [
  "get",
  "/v1/orgs/current/teams/{teamID}/plugins",
] as const
const TEAM_RAG_SOURCES_KEY = [
  "get",
  "/v1/orgs/current/teams/{teamID}/rag-sources",
] as const

export function TeamProvisioningSection({ teamId }: { teamId: string }) {
  const isAdmin = useIsAdmin()
  return (
    <div className="flex flex-col gap-8">
      <TeamPluginsSection teamId={teamId} readOnly={!isAdmin} />
      {isAdmin ? <TeamKnowledgeSourcesSection teamId={teamId} /> : null}
    </div>
  )
}

function TeamPluginsSection({
  teamId,
  readOnly,
}: {
  teamId: string
  readOnly: boolean
}) {
  const queryClient = useQueryClient()

  const pluginsQuery = $api.useQuery("get", "/v1/plugins")
  const teamPluginsQuery = $api.useQuery(
    "get",
    "/v1/orgs/current/teams/{teamID}/plugins",
    { params: { path: { teamID: teamId } } }
  )
  const enableMutation = $api.useMutation(
    "post",
    "/v1/orgs/current/teams/{teamID}/plugins"
  )
  const disableMutation = $api.useMutation(
    "delete",
    "/v1/orgs/current/teams/{teamID}/plugins/{pluginID}"
  )

  const plugins = useMemo(
    () =>
      (pluginsQuery.data ?? []).filter(
        (plugin) =>
          plugin.installed === true &&
          (isTeamProvisionable(plugin) || plugin.auto_install === true)
      ),
    [pluginsQuery.data]
  )
  const enabledIds = useMemo(
    () => enabledIdSet(teamPluginsQuery.data?.data),
    [teamPluginsQuery.data?.data]
  )
  const isBusy = enableMutation.isPending || disableMutation.isPending
  const isLoading = pluginsQuery.isLoading || teamPluginsQuery.isLoading

  function toggle(plugin: ApiPlugin, on: boolean) {
    const id = plugin.id
    if (!id) return
    const name = pluginName(plugin)
    const options = {
      onSuccess: () => {
        toast.success(`${on ? "Enabled" : "Disabled"} ${name} for this team`)
        queryClient.invalidateQueries({ queryKey: TEAM_PLUGINS_KEY })
      },
      onError: (err: unknown) =>
        toast.danger(
          extractErrorMessage(
            err,
            `Could not ${on ? "enable" : "disable"} plugin`
          )
        ),
    }
    if (on) {
      enableMutation.mutate(
        { params: { path: { teamID: teamId } }, body: { plugin_id: id } },
        options
      )
    } else {
      disableMutation.mutate(
        { params: { path: { teamID: teamId, pluginID: id } } },
        options
      )
    }
  }

  const enabledPlugins = useMemo(
    () =>
      plugins.filter(
        (plugin) =>
          plugin.auto_install === true || isProvisioned(plugin.id, enabledIds)
      ),
    [plugins, enabledIds]
  )

  if (readOnly) {
    return (
      <section className="flex flex-col gap-3">
        <SectionHeader
          title="Plugins"
          description="Plugins this team's agents can use."
        />
        <div className="overflow-hidden rounded-2xl border border-border bg-surface">
          {isLoading ? (
            <ProvisioningSkeleton />
          ) : enabledPlugins.length === 0 ? (
            <EmptyProvisioningRow text="No plugins are enabled for this team yet." />
          ) : (
            enabledPlugins.map((plugin, index) => (
              <ProvisioningRow
                key={plugin.id ?? index}
                readOnly
                last={index === enabledPlugins.length - 1}
                icon={<PluginLogoTile plugin={plugin} />}
                title={pluginName(plugin)}
                subtitle={plugin.description || "No description"}
                on
                label={`${pluginName(plugin)} is enabled for this team`}
              />
            ))
          )}
        </div>
      </section>
    )
  }

  return (
    <section className="flex flex-col gap-3">
      <SectionHeader
        title="Plugins"
        description="Plugins enabled here are available to every agent in this team."
      />
      <div className="overflow-hidden rounded-2xl border border-border bg-surface">
        {isLoading ? (
          <ProvisioningSkeleton />
        ) : plugins.length === 0 ? (
          <EmptyProvisioningRow text="No plugins are installed for this workspace yet." />
        ) : (
          plugins.map((plugin, index) => {
            const alwaysOn = plugin.auto_install === true
            const on = alwaysOn || isProvisioned(plugin.id, enabledIds)
            return (
              <ProvisioningRow
                key={plugin.id ?? index}
                last={index === plugins.length - 1}
                icon={<PluginLogoTile plugin={plugin} />}
                title={pluginName(plugin)}
                subtitle={plugin.description || "No description"}
                on={on}
                disabled={alwaysOn || isBusy || !plugin.id}
                label={
                  alwaysOn
                    ? `${pluginName(plugin)} is always enabled for every team`
                    : `${on ? "Disable" : "Enable"} ${pluginName(plugin)} for this team`
                }
                onChange={(selected) => toggle(plugin, selected)}
              />
            )
          })
        )}
      </div>
    </section>
  )
}

function TeamKnowledgeSourcesSection({ teamId }: { teamId: string }) {
  const queryClient = useQueryClient()

  const sourcesQuery = $api.useQuery("get", "/v1/rag/sources", {
    params: { query: { page_size: 100 } },
  })
  const connectionsQuery = $api.useQuery("get", "/v1/connections", {
    params: { query: { limit: 100 } },
  })
  const teamSourcesQuery = $api.useQuery(
    "get",
    "/v1/orgs/current/teams/{teamID}/rag-sources",
    { params: { path: { teamID: teamId } } }
  )
  const grantMutation = $api.useMutation(
    "post",
    "/v1/orgs/current/teams/{teamID}/rag-sources"
  )
  const revokeMutation = $api.useMutation(
    "delete",
    "/v1/orgs/current/teams/{teamID}/rag-sources/{sourceID}"
  )

  const sources = useMemo(
    () => sourcesQuery.data?.data ?? [],
    [sourcesQuery.data?.data]
  )
  const connectionsById = useMemo(() => {
    const map = new Map<string, Connection>()
    for (const conn of connectionsQuery.data?.data ?? []) {
      if (conn.id) map.set(conn.id, conn)
    }
    return map
  }, [connectionsQuery.data?.data])
  const enabledIds = useMemo(
    () => enabledIdSet(teamSourcesQuery.data?.data),
    [teamSourcesQuery.data?.data]
  )
  const isBusy = grantMutation.isPending || revokeMutation.isPending
  const isLoading =
    sourcesQuery.isLoading ||
    connectionsQuery.isLoading ||
    teamSourcesQuery.isLoading

  function toggle(source: RagSource, on: boolean) {
    const id = source.id
    if (!id) return
    const name = source.name || "source"
    const options = {
      onSuccess: () => {
        toast.success(`${on ? "Granted" : "Revoked"} ${name} for this team`)
        queryClient.invalidateQueries({ queryKey: TEAM_RAG_SOURCES_KEY })
      },
      onError: (err: unknown) =>
        toast.danger(
          extractErrorMessage(
            err,
            `Could not ${on ? "grant" : "revoke"} knowledge source`
          )
        ),
    }
    if (on) {
      grantMutation.mutate(
        { params: { path: { teamID: teamId } }, body: { rag_source_id: id } },
        options
      )
    } else {
      revokeMutation.mutate(
        { params: { path: { teamID: teamId, sourceID: id } } },
        options
      )
    }
  }

  return (
    <section className="flex flex-col gap-3">
      <SectionHeader
        title="Knowledge sources"
        description="Choose which knowledge sources this team can search."
      />
      <div className="overflow-hidden rounded-2xl border border-border bg-surface">
        {isLoading ? (
          <ProvisioningSkeleton />
        ) : sources.length === 0 ? (
          <EmptyProvisioningRow text="No knowledge sources exist for this workspace yet." />
        ) : (
          sources.map((source, index) => {
            const on = isProvisioned(source.id, enabledIds)
            const provider = providerMeta(
              deriveProvider(source, connectionsById)
            )
            return (
              <ProvisioningRow
                key={source.id ?? index}
                last={index === sources.length - 1}
                icon={
                  <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-default text-muted">
                    <ProviderIcon icon={provider.icon} className="h-4 w-4" />
                  </span>
                }
                title={source.name || "Untitled source"}
                subtitle={provider.label}
                on={on}
                disabled={isBusy || !source.id}
                label={`${on ? "Revoke" : "Grant"} ${source.name || "source"} for this team`}
                onChange={(selected) => toggle(source, selected)}
              />
            )
          })
        )}
      </div>
    </section>
  )
}
