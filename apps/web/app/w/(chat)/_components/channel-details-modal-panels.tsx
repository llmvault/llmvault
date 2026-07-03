"use client"

import { Button, Chip, Input, Spinner } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import type { components } from "@/lib/api/schema"
import { cn } from "@/lib/utils"
import type { ChannelResponse } from "@/app/w/(chat)/_lib/chat-cache"
import { channelDisplayName } from "@/app/w/(chat)/_lib/sidebar-data"
import { AgentSelect } from "@/components/agent-select"

type ChannelMember = components["schemas"]["channelMemberResponse"]
type OrgMember = components["schemas"]["orgMemberResponse"]
type AgentListItem = components["schemas"]["agentListItem"]

export function TabButton({
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

export function AboutPanel({
  channel,
  agents,
  agentsLoading,
  agentSaving,
  channelAgentID,
  copied,
  creator,
  editingName,
  nameDraft,
  nameInvalid,
  nameUnchanged,
  renamePending,
  onAgentChange,
  onCancelName,
  onCopyID,
  onEditName,
  onNameChange,
  onSaveName,
}: {
  channel: ChannelResponse | null | undefined
  agents: AgentListItem[]
  agentsLoading: boolean
  agentSaving: boolean
  channelAgentID: string
  copied: boolean
  creator: string
  editingName: boolean
  nameDraft: string
  nameInvalid: boolean
  nameUnchanged: boolean
  renamePending: boolean
  onAgentChange: (agentID: string) => void
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
      <div className="flex items-start gap-4 border-t border-border px-5 py-4">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <p className="text-sm font-medium">Agent</p>
            {agentSaving ? <Spinner size="sm" /> : null}
          </div>
          <p className="mt-1 text-sm text-muted">
            Mentions in this channel are handled by this agent.
          </p>
          <div className="mt-2">
            <AgentSelect
              agents={agents}
              selectedAgentID={channelAgentID}
              isLoading={agentsLoading}
              onChange={onAgentChange}
            />
          </div>
        </div>
      </div>
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

export function MembersPanel({
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

export function creatorForChannel(
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
