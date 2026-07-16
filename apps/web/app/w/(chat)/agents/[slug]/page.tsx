"use client"

import { use, useCallback, useMemo } from "react"
import NextLink from "next/link"
import { useQueryClient } from "@tanstack/react-query"
import { Skeleton, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { IntegrationLogo } from "@/components/integration-logo"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { queryKeys } from "@/lib/api/query-keys"
import { AgentAvatar } from "../_agent-avatar"
import { availableModelIds } from "@/app/w/(chat)/_lib/model-options"
import {
  AGENT_CATALOG_QUERY_KEY,
  INSTALLED_AGENTS_QUERY_KEY,
  agentDescription,
  agentName,
  normalizeAgentSandboxImage,
  normalizeAgentSandboxSize,
  type AgentSandboxImage,
  type AgentSandboxSize,
  type CatalogAgent,
  type InstalledAgent,
  type Team,
} from "../_lib"
import {
  AgentSettingsSection,
  SandboxImageSection,
  SandboxSizeSection,
} from "./_agent-settings-section"
import { AgentTeamsSection } from "./_agent-teams-section"

export default function AgentDetailPage({
  params,
}: {
  params: Promise<{ slug: string }>
}) {
  const { slug } = use(params)
  const queryClient = useQueryClient()
  const agentQuery = $api.useQuery("get", "/v1/agents/catalog/{slug}", {
    params: { path: { slug } },
  })
  const modelsQuery = $api.useQuery("get", "/v1/agents/models")
  const teamsQuery = $api.useQuery("get", "/v1/orgs/current/teams", {
    params: { query: { limit: 100 } },
  })
  const agent = agentQuery.data as CatalogAgent | undefined
  const installedID = agent?.installed_agent_id ?? ""
  const installedQuery = $api.useQuery(
    "get",
    "/v1/agents/{id}",
    { params: { path: { id: installedID } } },
    { enabled: Boolean(installedID) }
  )
  const installed = installedQuery.data as InstalledAgent | undefined
  const updateModel = $api.useMutation("patch", "/v1/agents/{id}/model")
  const update = $api.useMutation("patch", "/v1/agents/{id}")
  const models = useMemo(
    () => availableModelIds(modelsQuery.data ?? []),
    [modelsQuery.data]
  )
  const teams = (teamsQuery.data?.data ?? []) as Team[]
  const refresh = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: AGENT_CATALOG_QUERY_KEY })
    queryClient.invalidateQueries({ queryKey: INSTALLED_AGENTS_QUERY_KEY })
    queryClient.invalidateQueries({ queryKey: queryKeys.agentCatalog() })
    queryClient.invalidateQueries({ queryKey: queryKeys.agent() })
  }, [queryClient])
  const mutate = (body: Record<string, string>, success: string) =>
    installedID &&
    update.mutate(
      { params: { path: { id: installedID } }, body },
      {
        onSuccess: () => {
          toast.success(success)
          refresh()
        },
        onError: (error) =>
          toast.danger(extractErrorMessage(error, "Could not update agent")),
      }
    )
  if (agentQuery.isLoading) return <DetailSkeleton />
  if (!agent) return <NotFoundState />
  const required = agent.required_connections ?? []
  return (
    <div className="flex flex-col gap-8">
      <NextLink
        href="/w/agents"
        className="flex w-fit items-center gap-2 text-sm text-muted"
      >
        <AppIcon icon="arrow-left" className="h-4 w-4" />
        Agents
      </NextLink>
      <header className="flex items-center gap-3">
        <AgentAvatar agent={agent} size="lg" />
        <div>
          <h1 className="text-lg font-semibold">{agentName(agent)}</h1>
          <p className="mt-1 text-sm text-muted">{agentDescription(agent)}</p>
        </div>
      </header>
      {!agent.is_default ? (
        <AgentTeamsSection
          slug={slug}
          agent={agent}
          teams={teams}
          isLoading={teamsQuery.isLoading}
          onChanged={refresh}
        />
      ) : null}
      {installed ? (
        <div className="flex flex-col gap-6">
          <AgentSettingsSection
            availableModels={models}
            selectedModel={installed.model || agent.model || models[0] || ""}
            isBusy={updateModel.isPending}
            onModelChange={(model) =>
              updateModel.mutate(
                { params: { path: { id: installedID } }, body: { model } },
                { onSuccess: refresh }
              )
            }
          />
          <SandboxImageSection
            selectedSandboxImage={normalizeAgentSandboxImage(
              installed.sandbox_image
            )}
            isBusy={update.isPending}
            onSandboxImageChange={(value: AgentSandboxImage) =>
              mutate({ sandbox_image: value }, "Image template updated")
            }
          />
          <SandboxSizeSection
            selectedSandboxSize={normalizeAgentSandboxSize(
              installed.sandbox_size
            )}
            isBusy={update.isPending}
            onSandboxSizeChange={(value: AgentSandboxSize) =>
              mutate({ sandbox_size: value }, "Sandbox size updated")
            }
          />
        </div>
      ) : null}
      <section className="flex flex-col gap-3">
        <h2 className="text-sm font-semibold">Required connections</h2>
        <div className="bg-card divide-y divide-border overflow-hidden rounded-xl border border-border">
          {required.length ? (
            required.map((item) => (
              <div key={item.provider} className="flex items-center gap-3 p-3">
                <IntegrationLogo provider={item.provider ?? ""} size={32} />
                <span className="text-sm font-medium">{item.provider}</span>
              </div>
            ))
          ) : (
            <div className="p-4 text-sm text-muted">
              No required connections.
            </div>
          )}
        </div>
      </section>
    </div>
  )
}

function DetailSkeleton() {
  return (
    <div className="flex flex-col gap-8">
      <Skeleton className="h-4 w-20" />
      <Skeleton className="h-16 w-full" />
      <Skeleton className="h-44 w-full" />
    </div>
  )
}
function NotFoundState() {
  return (
    <div className="bg-card rounded-xl border border-border p-10 text-center">
      <p className="text-sm font-medium">Agent not found</p>
      <NextLink
        href="/w/agents"
        className="mt-3 inline-block text-sm text-muted"
      >
        Back to agents
      </NextLink>
    </div>
  )
}
