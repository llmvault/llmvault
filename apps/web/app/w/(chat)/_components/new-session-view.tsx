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
  markOptimisticEventFailed,
  optimisticThinkingEvent,
  optimisticUserEvent,
  removeSessionFromChannelCache,
  removeSessionEvent,
  replaceOptimisticEvent,
  replaceOptimisticSessionID,
  seedSessionDetail,
  seedSessionEvents,
} from "@/app/w/(chat)/_lib/chat-cache"
import {
  agentDisplayName,
  agentIcon,
  agentModel,
  channelRouteSlug,
  sessionDisplayName,
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
  const fallbackAgent = AGENTS[0]
  const defaultModelId =
    agentModel(selectedAgent) ?? fallbackAgent.defaultModelId
  const [selectedModelID, setSelectedModelID] = useState<string | null>(null)
  const modelId = selectedModelID ?? defaultModelId
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

  const createFirstSession = async (text: string, effort: string) => {
    if (!activeChannel?.id) {
      toast.danger("Select a channel first")
      return false
    }

    const tempSessionID = `tmp_${crypto.randomUUID()}`
    const title = sessionTitle(text)
    const now = new Date().toISOString()
    const tempSession = {
      id: tempSessionID,
      channel_id: activeChannel.id,
      agent_id: selectedAgent?.id,
      model: modelId,
      name: title,
      status: "creating",
      created_at: now,
      updated_at: now,
      last_activity_at: now,
    }
    const previewSession: ChatSession = {
      title,
      agentId: selectedAgent?.id ?? fallbackAgent.id,
      agentName: selectedAgent ? agentDisplayName(selectedAgent) : undefined,
      agentIcon: agentIcon(selectedAgent),
      modelId,
    }

    seedSessionDetail(queryClient, tempSession)
    insertSessionIntoChannelCache(queryClient, tempSession)
    const optimisticMessage = optimisticUserEvent(tempSessionID, text)
    const optimisticThinking = optimisticThinkingEvent(tempSessionID)
    const optimisticMessageID =
      optimisticMessage.event_id ?? optimisticMessage.id ?? ""
    const optimisticThinkingID =
      optimisticThinking.event_id ?? optimisticThinking.id ?? ""
    seedSessionEvents(queryClient, tempSessionID, [
      optimisticMessage,
      optimisticThinking,
    ])
    onSessionCreated(
      channelRouteSlug(activeChannel),
      tempSessionID,
      previewSession
    )

    void createSession
      .mutateAsync({
        body: {
          channel_id: activeChannel.id,
          agent_id: selectedAgent?.id,
          text,
          model: modelId,
          reasoning_effort: effort.toLowerCase(),
          access_mode: "full",
        },
      })
      .then((response) => {
        const created = response.session
        if (!created?.id) {
          toast.danger("Session was created without an id")
          return
        }
        seedSessionDetail(queryClient, created)
        insertSessionIntoChannelCache(queryClient, created)
        removeSessionFromChannelCache(
          queryClient,
          activeChannel.id,
          tempSessionID
        )
        replaceOptimisticSessionID(queryClient, tempSessionID, created)
        if (response.event) {
          replaceOptimisticEvent(
            queryClient,
            created.id,
            optimisticMessageID,
            response.event
          )
        }
        removeSessionEvent(queryClient, created.id, optimisticThinkingID)
        onSessionCreated(
          channelRouteSlug(activeChannel),
          created.id,
          {
            title: sessionDisplayName(created),
            agentId: selectedAgent?.id ?? fallbackAgent.id,
            agentName: selectedAgent
              ? agentDisplayName(selectedAgent)
              : undefined,
            agentIcon: agentIcon(selectedAgent),
            modelId,
          },
          { replace: true }
        )
      })
      .catch((error) => {
        const message = extractErrorMessage(error, "Could not create session")
        markOptimisticEventFailed(
          queryClient,
          tempSessionID,
          optimisticMessageID,
          message
        )
        removeSessionEvent(queryClient, tempSessionID, optimisticThinkingID)
        toast.danger(message)
      })

    return true
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

function sessionTitle(text: string) {
  return text.length > 44 ? `${text.slice(0, 44).trimEnd()}...` : text
}
