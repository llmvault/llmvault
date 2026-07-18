"use client"

import { useMemo, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "@heroui/react"
import { $api } from "@/lib/api/hooks"
import { useTeamAgents } from "@/lib/api/team-agents"
import { extractErrorMessage } from "@/lib/api/error"
import { Composer } from "./composer"
import { LogoMark } from "@/components/logo"
import type { ChatSession } from "./shell"
import {
  CHAT_QUERY_STALE_TIME_MS,
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
import { agentDisplayName } from "@/app/w/(chat)/_lib/sidebar-data"

interface SessionViewProps {
  onSessionCreated: (
    sessionId: string,
    session?: ChatSession,
    options?: { replace?: boolean }
  ) => void
}

export function SessionView({ onSessionCreated }: SessionViewProps) {
  const queryClient = useQueryClient()
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
  const teams = useMemo(
    () => teamsQuery.data?.data ?? [],
    [teamsQuery.data?.data]
  )
  const [selectedTeamID, setSelectedTeamID] = useState<string | null>(null)
  const activeTeamID = selectedTeamID ?? teams[0]?.id ?? null
  const activeTeam = teams.find((team) => team.id === activeTeamID)
  const {
    agents,
    isLoading: agentsLoading,
    isError: agentsError,
  } = useTeamAgents(activeTeamID)
  const [selectedAgentID, setSelectedAgentID] = useState<string | null>(null)
  const selectedAgent =
    agents.find((agent) => agent.id === selectedAgentID) ?? agents[0]
  const modelSummaries = useMemo(
    () => agentModelsQuery.data ?? [],
    [agentModelsQuery.data]
  )
  const modelIds = useMemo(
    () => composerModelIds(modelSummaries),
    [modelSummaries]
  )
  const agentDefaultModelID = selectedAgent?.model?.trim()
  const defaultModelID =
    (agentDefaultModelID && modelIds.includes(agentDefaultModelID)
      ? agentDefaultModelID
      : modelIds[0]) ?? ""
  const [selectedModelID, setSelectedModelID] = useState<string | null>(null)
  const modelID =
    selectedModelID && modelIds.includes(selectedModelID)
      ? selectedModelID
      : defaultModelID
  const draftKey = `new:${activeTeamID ?? "root"}`
  const setComposerUploads = useSessionWorkspaceStore(
    (state) => state.setComposerUploads
  )
  const setAttachmentDescriptions = useSessionWorkspaceStore(
    (state) => state.setAttachmentDescriptions
  )
  const setComposerText = useSessionWorkspaceStore(
    (state) => state.setComposerText
  )

  const resetDraft = () => {
    setComposerUploads(draftKey, () => [])
    setAttachmentDescriptions(draftKey, () => ({}))
    setComposerText(draftKey, "")
  }

  const handleTeamChange = (teamID: string) => {
    setSelectedTeamID(teamID)
    setSelectedAgentID(null)
    setSelectedModelID(null)
    resetDraft()
  }

  const createFirstSession = async (
    text: string,
    attachments: ImageAttachmentMetadata[],
    effort: string
  ) => {
    if (!selectedAgent?.id) {
      toast.danger("Select an agent first")
      return false
    }
    const attachmentIDs = imageAttachmentIDs(attachments)
    try {
      const response = await createSession.mutateAsync({
        body: {
          agent_id: selectedAgent.id,
          text,
          ...(attachmentIDs.length ? { attachment_ids: attachmentIDs } : {}),
          model_definition: {
            model_id: modelID,
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
      invalidateSessionListQueries(queryClient)
      watchGeneratedSessionName(queryClient, created)
      onSessionCreated(created.id, undefined, { replace: true })
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
            <h1 className="text-2xl font-semibold tracking-tight text-foreground">
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
          modelId={modelID}
          sessionExists={false}
          spendVisible={false}
          teamSelectable
          team={activeTeam}
          teams={teams}
          teamsLoading={teamsQuery.isLoading}
          teamsError={teamsQuery.isError}
          onTeamChange={(team) => team.id && handleTeamChange(team.id)}
          agentSelectable
          agent={selectedAgent}
          agents={agents}
          agentsLoading={agentsLoading}
          agentsError={agentsError}
          onAgentChange={(agent) => {
            setSelectedAgentID(agent.id ?? null)
            setSelectedModelID(null)
            resetDraft()
          }}
          modelSelectable
          modelIds={modelIds}
          modelSummaries={modelSummaries}
          modelsLoading={agentModelsQuery.isLoading}
          modelsError={agentModelsQuery.isError}
          onModelChange={setSelectedModelID}
          isSubmitting={createSession.isPending}
          onSend={(text, attachments, _comments, effort) =>
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
