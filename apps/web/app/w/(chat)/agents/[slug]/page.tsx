"use client"

import { use, useCallback, useMemo, useState, type ReactNode } from "react"
import NextLink from "next/link"
import { useQueryClient } from "@tanstack/react-query"
import { Skeleton, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
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
import { AgentMemoriesSection } from "./_agent-memories-section"
import { AgentConnectionsField } from "../new/_connections-field"

type AgentDetailTab = "overview" | "memories"

export default function AgentDetailPage({
  params,
}: {
  params: Promise<{ slug: string }>
}) {
  const { slug } = use(params)
  const [activeTab, setActiveTab] = useState<AgentDetailTab>("overview")
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
  const requiredProviders = required
    .map((connection) => connection.provider)
    .filter((provider): provider is string => Boolean(provider))
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
      {installed ? (
        <>
          <div
            role="tablist"
            aria-label="Agent details"
            className="flex gap-6 border-b border-border"
          >
            <DetailTab
              id="overview"
              activeTab={activeTab}
              onSelect={setActiveTab}
            >
              Overview
            </DetailTab>
            <DetailTab
              id="memories"
              activeTab={activeTab}
              onSelect={setActiveTab}
            >
              Memories
            </DetailTab>
          </div>

          {activeTab === "overview" ? (
            <div
              id="agent-overview-panel"
              role="tabpanel"
              aria-labelledby="agent-overview-tab"
              className="flex flex-col gap-6"
            >
              {!agent.is_default ? (
                <AgentTeamsSection
                  slug={slug}
                  agent={agent}
                  teams={teams}
                  isLoading={teamsQuery.isLoading}
                  onChanged={refresh}
                />
              ) : null}
              <AgentSettingsSection
                availableModels={models}
                selectedModel={
                  installed.model || agent.model || models[0] || ""
                }
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
              <AgentConnectionsField
                teamId={installed.team_id ?? ""}
                value={installed.connection_mcp_tool_deny ?? {}}
                disabled={update.isPending}
                lockedProviders={requiredProviders}
                onChange={(connectionMCPToolDeny) =>
                  update.mutate(
                    {
                      params: { path: { id: installedID } },
                      body: {
                        connection_mcp_tool_deny: connectionMCPToolDeny,
                      },
                    },
                    {
                      onSuccess: () => {
                        toast.success("Agent connections updated")
                        refresh()
                      },
                      onError: (error) =>
                        toast.danger(
                          extractErrorMessage(error, "Could not update agent")
                        ),
                    }
                  )
                }
              />
            </div>
          ) : (
            <div
              id="agent-memories-panel"
              role="tabpanel"
              aria-labelledby="agent-memories-tab"
            >
              <AgentMemoriesSection agentId={installedID} />
            </div>
          )}
        </>
      ) : !agent.is_default ? (
        <AgentTeamsSection
          slug={slug}
          agent={agent}
          teams={teams}
          isLoading={teamsQuery.isLoading}
          onChanged={refresh}
        />
      ) : null}
    </div>
  )
}

function DetailTab({
  id,
  activeTab,
  onSelect,
  children,
}: {
  id: AgentDetailTab
  activeTab: AgentDetailTab
  onSelect: (tab: AgentDetailTab) => void
  children: ReactNode
}) {
  const selected = id === activeTab
  const selectAndFocus = (tab: AgentDetailTab) => {
    onSelect(tab)
    document.getElementById(`agent-${tab}-tab`)?.focus()
  }
  return (
    <button
      id={`agent-${id}-tab`}
      type="button"
      role="tab"
      aria-selected={selected}
      aria-controls={`agent-${id}-panel`}
      tabIndex={selected ? 0 : -1}
      onClick={() => onSelect(id)}
      onKeyDown={(event) => {
        if (event.key === "ArrowLeft" || event.key === "Home") {
          event.preventDefault()
          selectAndFocus("overview")
        }
        if (event.key === "ArrowRight" || event.key === "End") {
          event.preventDefault()
          selectAndFocus("memories")
        }
      }}
      className={`-mb-px border-b-2 px-1 pb-2 text-sm font-medium transition-colors ${
        selected
          ? "border-foreground text-foreground"
          : "text-muted-foreground border-transparent hover:text-foreground"
      }`}
    >
      {children}
    </button>
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
