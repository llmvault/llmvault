"use client"

import { useMemo, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "@heroui/react"
import { $api } from "@/lib/api/hooks"
import { useTeamAgents } from "@/lib/api/team-agents"
import { extractErrorMessage } from "@/lib/api/error"
import { Composer } from "@/app/w/(chat)/_components/composer"
import { LogoMark } from "@/components/logo"
import type { ChatSession } from "@/app/w/(chat)/_components/shell"
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
  const teamsQuery = $api.useQuery(
    "get",
    "/v1/orgs/current/teams",
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
  const teams = useMemo(
    () => teamsQuery.data?.data ?? [],
    [teamsQuery.data?.data]
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
  const [selectedTeamID, setSelectedTeamID] = useState<string | null>(null)
  const [selectedChannelChoice, setSelectedChannelChoice] = useState<{
    routeSlug: string
    channelID: string
  } | null>(null)
  const selectedRouteSlug = channelSlug ?? ""
  const activeTeamID =
    selectedTeamID ?? defaultChannel?.team_id?.trim() ?? teams[0]?.id ?? null
  const activeTeam = teams.find((team) => team.id === activeTeamID)
  const teamChannels = useMemo(
    () => channels.filter((channel) => channel.team_id === activeTeamID),
    [channels, activeTeamID]
  )
  const selectedChannel =
    selectedChannelChoice?.routeSlug === selectedRouteSlug
      ? teamChannels.find(
          (channel) => channel.id === selectedChannelChoice.channelID
        )
      : undefined
  const teamDefaultChannel =
    teamChannels.find((channel) => channel.is_default) ?? teamChannels[0]
  const activeChannel =
    selectedChannel ??
    (defaultChannel?.team_id === activeTeamID
      ? defaultChannel
      : teamDefaultChannel)

  const {
    agents,
    isLoading: agentsLoading,
    isError: agentsError,
  } = useTeamAgents(activeTeamID)
  const [selectedAgentID, setSelectedAgentID] = useState<string | null>(null)
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
  const agentDefaultModelId = selectedAgent?.model?.trim()
  const defaultModelId =
    (agentDefaultModelId && modelIds.includes(agentDefaultModelId)
      ? agentDefaultModelId
      : modelIds[0]) ?? ""
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

  const resetDraftAttachments = () => {
    setComposerUploads(draftKey, () => [])
    setAttachmentDescriptions(draftKey, () => ({}))
  }

  const handleTeamChange = (teamID: string) => {
    setSelectedTeamID(teamID)
    const nextTeamChannels = channels.filter(
      (channel) => channel.team_id === teamID
    )
    const nextChannel =
      nextTeamChannels.find((channel) => channel.is_default) ??
      nextTeamChannels[0]
    setSelectedChannelChoice(
      nextChannel?.id
        ? { routeSlug: selectedRouteSlug, channelID: nextChannel.id }
        : null
    )
    setSelectedAgentID(null)
    setSelectedModelID(null)
    resetDraftAttachments()
  }

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
          <LogoMark className="h-16 w-16" />
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
          teamSelectable
          team={activeTeam}
          teams={teams}
          teamsLoading={teamsQuery.isLoading}
          teamsError={teamsQuery.isError}
          onTeamChange={(team) => {
            if (team.id) handleTeamChange(team.id)
          }}
          channelSelectable
          channel={activeChannel}
          channels={teamChannels}
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
            resetDraftAttachments()
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
              : "Ask an agent to do something..."
          }
        />
      </div>
      <div className="min-h-0 flex-1" />
    </div>
  )
}

function timeGreeting() {
  const hour = new Date().getHours()
  if (hour < 12) return "Good morning"
  if (hour < 18) return "Good afternoon"
  return "Good evening"
}
