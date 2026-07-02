"use client"

import { useMemo, useState } from "react"
import { usePathname, useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Chip, Input, Modal, Spinner, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"
import { cn } from "@/lib/utils"
import {
  CHAT_QUERY_STALE_TIME_MS,
  patchChannelInChatCaches,
  type ChannelResponse,
} from "@/app/w/(chat)/_lib/chat-cache"
import {
  channelDisplayName,
  channelRouteSlug,
} from "@/app/w/(chat)/_lib/sidebar-data"

type ChannelMember = components["schemas"]["channelMemberResponse"]
type OrgMember = components["schemas"]["orgMemberResponse"]
type DetailTab = "about" | "members"

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
                ) : (
                  <MembersPanel
                    members={members}
                    membersByID={membersByID}
                    loading={channelQuery.isLoading}
                  />
                )}
              </div>
            </div>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}

function TabButton({
  active,
  label,
  onClick,
}: {
  active: boolean
  label: string
  onClick: () => void
}) {
  return (
    <button
      type="button"
      role="tab"
      aria-selected={active}
      onClick={onClick}
      className={cn(
        "border-b-2 px-0 pb-3 text-sm font-medium transition-colors",
        active
          ? "border-accent text-foreground"
          : "border-transparent text-muted hover:text-foreground"
      )}
    >
      {label}
    </button>
  )
}

function AboutPanel({
  channel,
  copied,
  creator,
  editingName,
  nameDraft,
  nameInvalid,
  nameUnchanged,
  renamePending,
  onCancelName,
  onCopyID,
  onEditName,
  onNameChange,
  onSaveName,
}: {
  channel: ChannelResponse | null | undefined
  copied: boolean
  creator: string
  editingName: boolean
  nameDraft: string
  nameInvalid: boolean
  nameUnchanged: boolean
  renamePending: boolean
  onCancelName: () => void
  onCopyID: () => void
  onEditName: () => void
  onNameChange: (value: string) => void
  onSaveName: () => void
}) {
  return (
    <div className="rounded-xl border border-border bg-surface">
      <ChannelNameRow
        channel={channel}
        editing={editingName}
        invalid={nameInvalid}
        nameDraft={nameDraft}
        saving={renamePending}
        unchanged={nameUnchanged}
        onCancel={onCancelName}
        onChange={onNameChange}
        onEdit={onEditName}
        onSave={onSaveName}
      />
      <DetailRow
        title="Description"
        value={channel?.description?.trim() || "No description"}
      />
      <DetailRow title="Created" value={createdLabel(channel, creator)} />
      <div className="flex items-center gap-3 border-t border-border px-5 py-4">
        <div className="min-w-0 flex-1">
          <p className="text-sm font-medium">Channel ID</p>
          <p className="mt-1 truncate font-mono text-sm text-muted">
            {channel?.id ?? "Unavailable"}
          </p>
        </div>
        <Button
          variant="ghost"
          size="sm"
          isIconOnly
          aria-label="Copy channel ID"
          isDisabled={!channel?.id}
          onPress={onCopyID}
        >
          <AppIcon
            icon={copied ? "check" : "copy"}
            className="h-4 w-4 text-muted"
          />
        </Button>
      </div>
    </div>
  )
}

function ChannelNameRow({
  channel,
  editing,
  invalid,
  nameDraft,
  saving,
  unchanged,
  onCancel,
  onChange,
  onEdit,
  onSave,
}: {
  channel: ChannelResponse | null | undefined
  editing: boolean
  invalid: boolean
  nameDraft: string
  saving: boolean
  unchanged: boolean
  onCancel: () => void
  onChange: (value: string) => void
  onEdit: () => void
  onSave: () => void
}) {
  return (
    <div className="flex items-start gap-4 px-5 py-4">
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium">Channel name</p>
        {editing ? (
          <div className="mt-2 flex flex-col gap-2">
            <Input
              autoFocus
              value={nameDraft}
              disabled={saving}
              aria-invalid={invalid || undefined}
              aria-describedby={invalid ? "channel-name-error" : undefined}
              onChange={(event) => onChange(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter") onSave()
                if (event.key === "Escape") onCancel()
              }}
            />
            {invalid ? (
              <p id="channel-name-error" className="text-sm text-danger">
                Enter a channel name.
              </p>
            ) : null}
            <div className="flex justify-end gap-2">
              <Button
                variant="tertiary"
                size="sm"
                isDisabled={saving}
                onPress={onCancel}
              >
                Cancel
              </Button>
              <Button
                variant="primary"
                size="sm"
                isDisabled={invalid || unchanged || saving}
                onPress={onSave}
              >
                {saving ? <Spinner color="current" size="sm" /> : null}
                Save
              </Button>
            </div>
          </div>
        ) : (
          <p className="mt-1 text-sm text-muted">
            {channel ? `# ${channelDisplayName(channel)}` : "Loading..."}
          </p>
        )}
      </div>
      {!editing && channel?.id ? (
        <Button variant="ghost" size="sm" onPress={onEdit}>
          Edit
        </Button>
      ) : null}
    </div>
  )
}

function DetailRow({
  actionLabel,
  onAction,
  title,
  value,
}: {
  actionLabel?: string
  onAction?: () => void
  title: string
  value: string
}) {
  return (
    <div className="flex items-start gap-4 border-t border-border px-5 py-4 first:border-t-0">
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium">{title}</p>
        <p className="mt-1 text-sm text-muted">{value}</p>
      </div>
      {actionLabel && onAction ? (
        <Button variant="ghost" size="sm" onPress={onAction}>
          {actionLabel}
        </Button>
      ) : null}
    </div>
  )
}

function MembersPanel({
  loading,
  members,
  membersByID,
}: {
  loading: boolean
  members: ChannelMember[]
  membersByID: Map<string, OrgMember>
}) {
  if (loading) {
    return (
      <div className="flex min-h-32 items-center justify-center rounded-xl border border-border bg-surface">
        <Spinner size="sm" />
      </div>
    )
  }

  if (!members.length) {
    return (
      <div className="rounded-xl border border-border bg-surface px-5 py-6 text-sm text-muted">
        No channel members returned.
      </div>
    )
  }

  return (
    <div className="rounded-xl border border-border bg-surface">
      {members.map((member, index) => {
        const orgMember = member.user_id
          ? membersByID.get(member.user_id)
          : undefined
        const label = memberLabel(member, orgMember)
        return (
          <div
            key={member.user_id ?? index}
            className="flex items-center gap-3 border-t border-border px-5 py-3 first:border-t-0"
          >
            <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-default text-sm font-medium">
              {label.slice(0, 1).toUpperCase()}
            </div>
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-medium">{label}</p>
              <p className="truncate text-sm text-muted">
                {orgMember?.email ?? member.user_id ?? "Unknown user"}
              </p>
            </div>
            {member.role ? (
              <Chip size="sm">{roleLabel(member.role)}</Chip>
            ) : null}
          </div>
        )
      })}
    </div>
  )
}

function memberLabel(member: ChannelMember, orgMember: OrgMember | undefined) {
  return (
    orgMember?.name?.trim() || orgMember?.email || member.user_id || "Member"
  )
}

function creatorForChannel(
  channel: ChannelResponse | null | undefined,
  membersByID: Map<string, OrgMember>
) {
  if (!channel?.created_by) return "Unknown"
  const member = membersByID.get(channel.created_by)
  return member?.name?.trim() || member?.email || channel.created_by
}

function createdLabel(
  channel: ChannelResponse | null | undefined,
  creator: string
) {
  const date = formatDate(channel?.created_at)
  if (date && creator !== "Unknown") return `${creator} on ${date}`
  if (date) return date
  if (creator !== "Unknown") return creator
  return "Unknown"
}

function roleLabel(role: string) {
  return titleCase(role.replaceAll("_", " "))
}

function titleCase(value: string) {
  return value
    .split(/[\s_-]+/)
    .filter(Boolean)
    .map((part) => `${part.slice(0, 1).toUpperCase()}${part.slice(1)}`)
    .join(" ")
}

function formatDate(value: string | undefined | null) {
  if (!value) return ""
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ""
  return new Intl.DateTimeFormat("en-NG", {
    month: "long",
    day: "numeric",
    year: "numeric",
  }).format(date)
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
