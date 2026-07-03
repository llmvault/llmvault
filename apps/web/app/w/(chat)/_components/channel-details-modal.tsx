"use client"

import { useMemo, useState } from "react"
import { usePathname, useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Modal, Spinner, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import {
  CHAT_QUERY_STALE_TIME_MS,
  patchChannelInChatCaches,
  type ChannelResponse,
} from "@/app/w/(chat)/_lib/chat-cache"
import {
  channelDisplayName,
  channelRouteSlug,
} from "@/app/w/(chat)/_lib/sidebar-data"
import { ChannelEnvironmentVariablesPanel } from "@/app/w/(chat)/_components/channel-environment-variables"
import {
  AboutPanel,
  creatorForChannel,
  MembersPanel,
  TabButton,
} from "@/app/w/(chat)/_components/channel-details-modal-panels"

type DetailTab = "about" | "members" | "env"

export function ChannelDetailsModal({
  channel,
  initialNameEdit = false,
  open,
  onOpenChange,
}: {
  channel: ChannelResponse | null
  initialNameEdit?: boolean
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  if (!open) return null

  return (
    <ChannelDetailsModalContent
      key={`${channel?.id ?? "channel"}:${initialNameEdit ? "edit" : "view"}`}
      channel={channel}
      initialNameEdit={initialNameEdit}
      onOpenChange={onOpenChange}
    />
  )
}

function ChannelDetailsModalContent({
  channel,
  initialNameEdit = false,
  onOpenChange,
}: {
  channel: ChannelResponse | null
  initialNameEdit?: boolean
  onOpenChange: (open: boolean) => void
}) {
  const router = useRouter()
  const pathname = usePathname()
  const queryClient = useQueryClient()
  const [tab, setTab] = useState<DetailTab>("about")
  const [copied, setCopied] = useState(false)
  const [editingName, setEditingName] = useState(initialNameEdit)
  const [nameDraft, setNameDraft] = useState(
    channel ? channelDisplayName(channel) : ""
  )
  const channelID = channel?.id ?? ""
  const channelQuery = $api.useQuery(
    "get",
    "/v1/channels/{id}",
    { params: { path: { id: channelID } } },
    {
      enabled: Boolean(channelID),
      retry: false,
      staleTime: CHAT_QUERY_STALE_TIME_MS,
    }
  )
  const orgMembersQuery = $api.useQuery(
    "get",
    "/v1/orgs/current/members",
    {},
    {
      retry: false,
      staleTime: CHAT_QUERY_STALE_TIME_MS,
    }
  )
  const agentsQuery = $api.useQuery(
    "get",
    "/v1/agents",
    { params: { query: { status: "active", limit: 100 } } },
    {
      retry: false,
      staleTime: CHAT_QUERY_STALE_TIME_MS,
    }
  )
  const agents = useMemo(
    () => agentsQuery.data?.data ?? [],
    [agentsQuery.data?.data]
  )
  const detailChannel = channelQuery.data?.channel ?? channel
  const members = channelQuery.data?.members ?? []
  const membersByID = useMemo(
    () =>
      new Map(
        (orgMembersQuery.data?.data ?? []).flatMap((member) =>
          member.user_id ? ([[member.user_id, member]] as const) : []
        )
      ),
    [orgMembersQuery.data?.data]
  )
  const memberCount = detailChannel?.member_count ?? members.length
  const channelName = detailChannel
    ? channelDisplayName(detailChannel)
    : "Channel"
  const renameChannel = $api.useMutation("patch", "/v1/channels/{id}")
  const updateChannelAgent = $api.useMutation("patch", "/v1/channels/{id}")
  const [pendingAgentID, setPendingAgentID] = useState<string | null>(null)
  const channelAgentID = updateChannelAgent.isPending
    ? (pendingAgentID ?? "")
    : (detailChannel?.default_agent_id ?? "")
  const trimmedName = nameDraft.trim()
  const nameInvalid = editingName && trimmedName.length === 0
  const nameUnchanged = trimmedName === channelName

  async function copyChannelID() {
    if (!channelID) return
    try {
      await navigator.clipboard.writeText(channelID)
      toast.success("Channel ID copied")
    } catch {
      toast.danger("Could not copy channel ID")
    }
    setCopied(true)
    window.setTimeout(() => setCopied(false), 1400)
  }

  function cancelNameEdit() {
    if (renameChannel.isPending) return
    setNameDraft(channelName)
    setEditingName(false)
  }

  function saveName() {
    if (!detailChannel?.id || nameInvalid || renameChannel.isPending) return
    if (nameUnchanged) {
      setEditingName(false)
      return
    }
    renameChannel.mutate(
      {
        params: { path: { id: detailChannel.id } },
        body: { name: trimmedName },
      },
      {
        onSuccess: (response) => {
          if (response.channel) {
            patchChannelInChatCaches(queryClient, response.channel)
            replaceCurrentChannelRoute(
              detailChannel,
              response.channel,
              pathname,
              router
            )
            setNameDraft(channelDisplayName(response.channel))
          }
          setEditingName(false)
          toast.success("Channel renamed")
        },
        onError: (error) =>
          toast.danger(extractErrorMessage(error, "Could not rename channel")),
      }
    )
  }

  function saveAgent(agentID: string) {
    if (!detailChannel?.id || updateChannelAgent.isPending) return
    if (!agentID || agentID === detailChannel.default_agent_id) return
    setPendingAgentID(agentID)
    updateChannelAgent.mutate(
      {
        params: { path: { id: detailChannel.id } },
        body: { default_agent_id: agentID },
      },
      {
        onSuccess: (response) => {
          if (response.channel) {
            patchChannelInChatCaches(queryClient, response.channel)
          }
          toast.success("Channel agent updated")
        },
        onError: (error) =>
          toast.danger(
            extractErrorMessage(error, "Could not update channel agent")
          ),
        onSettled: () => setPendingAgentID(null),
      }
    )
  }

  function close(nextOpen: boolean) {
    if (!nextOpen && renameChannel.isPending) return
    if (!nextOpen) setTab("about")
    if (!nextOpen) setEditingName(false)
    onOpenChange(nextOpen)
  }

  return (
    <Modal isOpen onOpenChange={close}>
      <Modal.Backdrop className="bg-background/80 backdrop-blur-sm">
        <Modal.Container placement="center" className="p-4">
          <Modal.Dialog className="flex h-[min(640px,calc(100vh-2rem))] w-full max-w-lg overflow-hidden p-0">
            <Modal.CloseTrigger />
            <div className="flex h-full min-h-0 flex-col">
              <div className="px-6 pt-6 pb-4">
                <div className="flex min-w-0 items-start gap-3 pr-10">
                  <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-default text-foreground">
                    <AppIcon icon="hash" className="h-5 w-5" />
                  </div>
                  <div className="min-w-0 flex-1">
                    <Modal.Heading>#{channelName}</Modal.Heading>
                    <p className="mt-1 text-sm text-muted">
                      {memberCount
                        ? `${memberCount} members`
                        : "No members yet"}
                    </p>
                  </div>
                </div>

                <div className="mt-5 flex flex-wrap gap-2">
                  <Button
                    variant="tertiary"
                    size="sm"
                    isDisabled={!channelID}
                    onPress={copyChannelID}
                  >
                    <AppIcon
                      icon={copied ? "check" : "copy"}
                      className="h-4 w-4"
                    />
                    Copy ID
                  </Button>
                  {channelQuery.isFetching ? (
                    <span className="flex items-center px-2 text-muted">
                      <Spinner size="sm" />
                    </span>
                  ) : null}
                </div>
              </div>

              <div
                role="tablist"
                className="flex gap-7 border-b border-border px-6"
              >
                <TabButton
                  active={tab === "about"}
                  label="About"
                  onClick={() => setTab("about")}
                />
                <TabButton
                  active={tab === "members"}
                  label={memberCount ? `Members ${memberCount}` : "Members"}
                  onClick={() => setTab("members")}
                />
                <TabButton
                  active={tab === "env"}
                  label="Env vars"
                  onClick={() => setTab("env")}
                />
              </div>

              <div className="min-h-0 flex-1 overflow-y-auto bg-surface-secondary/40 px-6 py-6">
                {channelQuery.isError ? (
                  <div className="mb-4 rounded-lg border border-danger/30 bg-danger/10 px-3 py-2 text-sm text-danger">
                    Could not refresh channel details.
                  </div>
                ) : null}
                {tab === "about" ? (
                  <AboutPanel
                    channel={detailChannel}
                    agents={agents}
                    agentsLoading={agentsQuery.isLoading}
                    channelAgentID={channelAgentID}
                    agentSaving={updateChannelAgent.isPending}
                    onAgentChange={saveAgent}
                    creator={creatorForChannel(detailChannel, membersByID)}
                    editingName={editingName}
                    nameDraft={nameDraft}
                    nameInvalid={nameInvalid}
                    nameUnchanged={nameUnchanged}
                    renamePending={renameChannel.isPending}
                    onCancelName={cancelNameEdit}
                    onCopyID={copyChannelID}
                    onEditName={() => {
                      setNameDraft(channelName)
                      setEditingName(true)
                    }}
                    onNameChange={setNameDraft}
                    onSaveName={saveName}
                    copied={copied}
                  />
                ) : tab === "members" ? (
                  <MembersPanel
                    members={members}
                    membersByID={membersByID}
                    loading={channelQuery.isLoading}
                  />
                ) : (
                  <ChannelEnvironmentVariablesPanel channelId={channelID} />
                )}
              </div>
            </div>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}

function replaceCurrentChannelRoute(
  original: ChannelResponse,
  renamed: ChannelResponse,
  pathname: string,
  router: ReturnType<typeof useRouter>
) {
  const oldPrefix = `/w/channels/${channelRouteSlug(original)}`
  if (pathname !== oldPrefix && !pathname.startsWith(`${oldPrefix}/`)) return
  const newPrefix = `/w/channels/${channelRouteSlug(renamed)}`
  const suffix = pathname.slice(oldPrefix.length)
  router.replace(`${newPrefix}${suffix}`)
}
