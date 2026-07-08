"use client"

import { useMemo, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "@heroui/react"
import { $api } from "@/lib/api/hooks"
import { useTeamAgents } from "@/lib/api/team-agents"
import { extractErrorMessage } from "@/lib/api/error"
import { Composer } from "@/app/w/(chat)/_components/composer"
import { LogoMark } from "@/components/logo"
import { PluginExamplePrompts } from "@/app/w/(chat)/_components/plugin-example-prompts"
import type { ChatSession } from "@/app/w/(chat)/_components/shell"
import { AGENTS } from "@/app/w/(chat)/_lib/agents"
import {
  CHAT_QUERY_STALE_TIME_MS,
  insertSessionIntoChannelCache,
  invalidateSessionListQueries,
  seedSessionDetail,
} from "@/app/w/(chat)/_lib/chat-cache"
import { composerModelIds } from "@/app/w/(chat)/_lib/model-options"
import {
  imageAttachmentIDs,
  type ImageAttachmentMetadata,
} from "@/app/w/(chat)/_lib/image-attachments"
import { useSessionWorkspaceStore } from "@/app/w/(chat)/_stores/session-workspace-store"
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

  // Agent options are the agents on the active channel's team — agents are team
  // members now, so those are exactly the ones that can run sessions here.
  const {
    agents,
    isLoading: agentsLoading,
    isError: agentsError,
  } = useTeamAgents(activeChannel?.team_id)
  const [selectedAgentID, setSelectedAgentID] = useState<string | null>(null)
  // Default to the channel's configured default agent, then the first team
  // agent. A prior explicit pick (selectedAgentID) wins while it's still one of
  // the team's agents; after switching channels it falls through to the new
  // channel's default.
  const selectedAgent =
    agents.find((agent) => agent.id === selectedAgentID) ??
    agents.find((agent) => agent.id === activeChannel?.default_agent_id) ??
    agents[0]
  const modelSummaries = useMemo(
    () => agentModelsQuery.data ?? [],
    [agentModelsQuery.data]
  )
  const modelIds = useMemo(
    () => composerModelIds(modelSummaries),
    [modelSummaries]
  )
  const fallbackAgent = AGENTS[0]
  const agentDefaultModelId = selectedAgent?.model?.trim()
  const defaultModelId =
    (agentDefaultModelId && modelIds.includes(agentDefaultModelId)
      ? agentDefaultModelId
      : modelIds[0]) ?? fallbackAgent.defaultModelId
  const [selectedModelID, setSelectedModelID] = useState<string | null>(null)
  const modelId =
    selectedModelID && modelIds.includes(selectedModelID)
      ? selectedModelID
      : defaultModelId
  const draftKey = `new:${channelSlug ?? "root"}`
  const setComposerUploads = useSessionWorkspaceStore(
    (state) => state.setComposerUploads
  )
  const setAttachmentDescriptions = useSessionWorkspaceStore(
    (state) => state.setAttachmentDescriptions
  )
  const setComposerText = useSessionWorkspaceStore(
    (state) => state.setComposerText
  )

  const createFirstSession = async (
    text: string,
    attachments: ImageAttachmentMetadata[],
    effort: string
  ) => {
    if (!activeChannel?.id) {
      toast.danger("Select a channel first")
      return false
    }

    const attachmentIDs = imageAttachmentIDs(attachments)
    try {
      const response = await createSession.mutateAsync({
        body: {
          channel_id: activeChannel.id,
          agent_id: selectedAgent?.id,
          text,
          ...(attachmentIDs.length
            ? { attachment_ids: attachmentIDs }
            : {}),
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
    <div className="flex h-full min-w-0 flex-col px-2">
      <div className="h-48 shrink-0" />
      <div className="flex w-full flex-col gap-8">
        <div className="flex flex-col items-center gap-4 text-center">
          <LogoMark className="h-12 w-12" />
          <div>
            <h1
              suppressHydrationWarning
              className="text-2xl font-semibold tracking-tight text-foreground"
            >
              {timeGreeting()}
            </h1>
            <p className="mt-1 text-base text-muted">
              What are we working on today?
            </p>
          </div>
        </div>
        <Composer
          sessionId={draftKey}
          agentId={selectedAgent?.id ?? ""}
          modelId={modelId}
          sessionExists={false}
          spendVisible={false}
          channelSelectable
          channel={activeChannel}
          channels={channels}
          channelsLoading={channelsQuery.isLoading}
          channelsError={channelsQuery.isError}
          onChannelChange={(channel) =>
            setSelectedChannelChoice(
              channel.id
                ? { routeSlug: selectedRouteSlug, channelID: channel.id }
                : null
            )
          }
          agentSelectable
          agent={selectedAgent}
          agents={agents}
          agentsLoading={agentsLoading}
          agentsError={agentsError}
          onAgentChange={(agent) => {
            setSelectedAgentID(agent.id ?? null)
            setSelectedModelID(null)
            // Draft uploads live in the previous agent's drive; they cannot
            // be attached to a session for a different agent.
            setComposerUploads(draftKey, () => [])
            setAttachmentDescriptions(draftKey, () => ({}))
          }}
          modelSelectable
          modelIds={modelIds}
          modelSummaries={modelSummaries}
          modelsLoading={agentModelsQuery.isLoading}
          modelsError={agentModelsQuery.isError}
          onModelChange={setSelectedModelID}
          isSubmitting={createSession.isPending}
          onSend={(text, attachments, _codeLineComments, effort) =>
            createFirstSession(text, attachments, effort)
          }
          placeholder={
            selectedAgent?.name
              ? `Ask ${agentDisplayName(selectedAgent)} to do something...`
              : fallbackAgent.placeholder
          }
        />
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto pt-6 pb-4">
        <PluginExamplePrompts
          agentId={selectedAgent?.id}
          onSelect={(prompt) => setComposerText(draftKey, prompt)}
        />
      </div>
    </div>
  )
}

function timeGreeting() {
  const hour = new Date().getHours()
  if (hour < 12) return "Good morning"
  if (hour < 18) return "Good afternoon"
  return "Good evening"
}
