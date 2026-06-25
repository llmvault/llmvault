"use client"

import { useMemo, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "@heroui/react"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { ChatComposer } from "@/app/w/(chat)/_components/chat-composer"
import type { ChatSession } from "@/app/w/(chat)/_components/shell"
import { AGENTS } from "@/app/w/(chat)/_lib/agents"
import {
  CHAT_QUERY_STALE_TIME_MS,
  insertSessionIntoChannelCache,
  invalidateSessionListQueries,
  seedSessionDetail,
} from "@/app/w/(chat)/_lib/chat-cache"
import {
  availableModelIds,
  newSessionModelIds,
} from "@/app/w/(chat)/_lib/model-options"
import { watchGeneratedSessionName } from "@/app/w/(chat)/_lib/session-name-updates"
import {
  agentDisplayName,
  channelRouteSlug,
} from "@/app/w/(chat)/_lib/sidebar-data"

interface SessionViewProps {
  channelSlug?: string
  onSessionCreated: (
    channelSlug: string,
    sessionId: string,
    session?: ChatSession,
    options?: { replace?: boolean }
  ) => void
}

export function SessionView({
  channelSlug,
  onSessionCreated,
}: SessionViewProps) {
  const queryClient = useQueryClient()
  const channelsQuery = $api.useQuery(
    "get",
    "/v1/channels",
    { params: { query: { limit: 100 } } },
    { retry: false, staleTime: CHAT_QUERY_STALE_TIME_MS }
  )
  const agentsQuery = $api.useQuery(
    "get",
    "/v1/agents",
    { params: { query: { status: "active", limit: 100 } } },
    { retry: false, staleTime: CHAT_QUERY_STALE_TIME_MS }
  )
  const agentCatalogQuery = $api.useQuery(
    "get",
    "/v1/agents/catalog",
    {},
    { retry: false, staleTime: CHAT_QUERY_STALE_TIME_MS }
  )
  const agentModelsQuery = $api.useQuery(
    "get",
    "/v1/agents/models",
    {},
    { retry: false, staleTime: CHAT_QUERY_STALE_TIME_MS }
  )
  const createSession = $api.useMutation("post", "/v1/sessions")
  const channels = useMemo(
    () => channelsQuery.data?.data ?? [],
    [channelsQuery.data?.data]
  )
  const routeChannel = useMemo(
    () =>
      channelSlug
        ? channels.find((channel) => channelRouteSlug(channel) === channelSlug)
        : undefined,
    [channelSlug, channels]
  )
  const defaultChannel =
    routeChannel ??
    channels.find((channel) => channel.is_default) ??
    channels[0]
  const agents = useMemo(
    () => agentsQuery.data?.data ?? [],
    [agentsQuery.data?.data]
  )
  const [selectedAgentID, setSelectedAgentID] = useState<string | null>(null)
  const selectedAgent =
    agents.find((agent) => agent.id === selectedAgentID) ??
    agents.find((agent) => agent.is_default) ??
    agents[0]
  const agentCatalog = useMemo(
    () => agentCatalogQuery.data ?? [],
    [agentCatalogQuery.data]
  )
  const modelSummaries = useMemo(
    () => agentModelsQuery.data ?? [],
    [agentModelsQuery.data]
  )
  const modelIds = useMemo(() => {
    const ids = newSessionModelIds(selectedAgent, agentCatalog)
    return ids.length ? ids : availableModelIds(modelSummaries)
  }, [agentCatalog, modelSummaries, selectedAgent])
  const fallbackAgent = AGENTS[0]
  const defaultModelId = modelIds[0] ?? fallbackAgent.defaultModelId
  const [selectedModelID, setSelectedModelID] = useState<string | null>(null)
  const modelId =
    selectedModelID && modelIds.includes(selectedModelID)
      ? selectedModelID
      : defaultModelId
  const [selectedChannelChoice, setSelectedChannelChoice] = useState<{
    routeSlug: string
    channelID: string
  } | null>(null)
  const selectedRouteSlug = channelSlug ?? ""
  const selectedChannel =
    selectedChannelChoice?.routeSlug === selectedRouteSlug
      ? channels.find(
          (channel) => channel.id === selectedChannelChoice.channelID
        )
      : undefined
  const activeChannel = selectedChannel ?? defaultChannel
  const draftKey = `new:${channelSlug ?? "root"}`

  const createFirstSession = async (text: string, effort: string) => {
    if (!activeChannel?.id) {
      toast.danger("Select a channel first")
      return false
    }

    try {
      const response = await createSession.mutateAsync({
        body: {
          channel_id: activeChannel.id,
          agent_id: selectedAgent?.id,
          text,
	          model_definition: {
	            model_id: modelId,
	            reasoning_effort: effort.toLowerCase(),
	          },
	        },
	      })

      const created = response.session
      if (!created?.id) {
        toast.danger("Session was created without an id")
        return false
      }

      seedSessionDetail(queryClient, created)
      insertSessionIntoChannelCache(queryClient, created)
      invalidateSessionListQueries(queryClient)
      watchGeneratedSessionName(queryClient, created)

      onSessionCreated(channelRouteSlug(activeChannel), created.id, undefined, {
        replace: true,
      })
      return true
    } catch (error) {
      toast.danger(extractErrorMessage(error, "Could not create session"))
      return false
    }
  }

  return (
    <div className="flex h-full min-w-0 items-center justify-center px-6 pb-16">
      <div className="w-full max-w-3xl">
        <ChatComposer
          channel={activeChannel}
          channels={channels}
          channelsLoading={channelsQuery.isLoading}
          channelsError={channelsQuery.isError}
          agent={selectedAgent}
          agents={agents}
          agentsLoading={agentsQuery.isLoading}
          agentsError={agentsQuery.isError}
          modelId={modelId}
          modelIds={modelIds}
          modelSummaries={modelSummaries}
          modelsLoading={
            agentCatalogQuery.isLoading || agentModelsQuery.isLoading
          }
          modelsError={agentCatalogQuery.isError || agentModelsQuery.isError}
          submitting={createSession.isPending}
          onChannelChange={(channel) =>
            setSelectedChannelChoice(
              channel.id
                ? { routeSlug: selectedRouteSlug, channelID: channel.id }
                : null
            )
          }
          onAgentChange={(agent) => {
            setSelectedAgentID(agent.id ?? null)
            setSelectedModelID(null)
          }}
          onModelChange={setSelectedModelID}
          onSend={createFirstSession}
          draftKey={draftKey}
          placeholder={
            selectedAgent?.name
              ? `Ask ${agentDisplayName(selectedAgent)} to do something...`
              : fallbackAgent.placeholder
          }
        />
      </div>
    </div>
  )
}
