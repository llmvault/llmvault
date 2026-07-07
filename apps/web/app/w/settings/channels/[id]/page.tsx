"use client"

import { use, useMemo, useState } from "react"
import NextLink from "next/link"
import { useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import {
  Button,
  Input,
  ListBox,
  Select,
  Skeleton,
  Spinner,
  Switch,
  TextArea,
  toast,
} from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { AgentSelect } from "@/components/agent-select"
import { ConfirmDialog } from "@/components/confirm-dialog"
import type { SidebarAgentResponse } from "@/app/w/(chat)/_lib/sidebar-data"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { CHANNEL_CATEGORIES, channelMemorySettings } from "@/lib/api/memory"
import { FormSection } from "@/app/w/(chat)/automations/_trigger-form-sections"
import { ChannelEnvironmentVariablesPanel } from "@/app/w/(chat)/_components/channel-environment-variables"
import { KnowledgeSourcesTab } from "./_knowledge-sources-tab"

type ChannelMember = {
  user_id?: string
  name?: string
  email?: string
  role?: string
}

type ChannelTab = "channel" | "members" | "knowledge" | "env"

export default function ChannelDetailPage({
  params,
}: {
  params: Promise<{ id: string }>
}) {
  const { id } = use(params)
  const router = useRouter()
  const queryClient = useQueryClient()
  const [tab, setTab] = useState<ChannelTab>("channel")
  const [confirmOpen, setConfirmOpen] = useState(false)

  const channelQuery = $api.useQuery("get", "/v1/channels/{id}", {
    params: { path: { id } },
  })
  const agentsQuery = $api.useQuery("get", "/v1/agents", {
    params: { query: { status: "active", limit: 200 } },
  })
  const updateChannel = $api.useMutation("patch", "/v1/channels/{id}")
  const deleteChannel = $api.useMutation("delete", "/v1/channels/{id}")

  const channel = channelQuery.data?.channel
  const members = useMemo(
    () => (channelQuery.data?.members ?? []) as ChannelMember[],
    [channelQuery.data?.members]
  )
  const agents = useMemo(
    () => (agentsQuery.data?.data ?? []) as SidebarAgentResponse[],
    [agentsQuery.data?.data]
  )

  const memorySettings = channelMemorySettings(channel)

  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [agentId, setAgentId] = useState("")
  const [visibility, setVisibility] = useState("public")
  const [category, setCategory] = useState("")
  const [memoryMission, setMemoryMission] = useState("")
  const [hydratedId, setHydratedId] = useState<string | null>(null)

  // The effective mission: the stored per-channel mission, or the category's
  // curated default when unset (what extraction actually uses then). The
  // textbox always shows the effective value so users see the mission in use
  // even when they never set one; clearing it resets to the default.
  const missionBaseline =
    memorySettings.memoryMission || memorySettings.defaultMemoryMission

  function resetForm() {
    setName(channel?.name ?? "")
    setDescription(channel?.description ?? "")
    setAgentId(channel?.default_agent_id ?? "")
    setVisibility(channel?.visibility ?? "public")
    const memory = channelMemorySettings(channel)
    setCategory(memory.category)
    setMemoryMission(memory.memoryMission || memory.defaultMemoryMission)
  }

  // Hydrate the form once from the loaded channel (and re-hydrate on route
  // switch). Adjusting state during render is React's recommended alternative
  // to a setState-in-effect here.
  if (channel && hydratedId !== (channel.id ?? null)) {
    setHydratedId(channel.id ?? null)
    resetForm()
  }

  const dirty =
    channel != null &&
    (name.trim() !== (channel.name ?? "") ||
      description !== (channel.description ?? "") ||
      agentId !== (channel.default_agent_id ?? "") ||
      visibility !== (channel.visibility ?? "public") ||
      category !== memorySettings.category ||
      memoryMission !== missionBaseline)
  const canSave = dirty && name.trim().length > 0 && !updateChannel.isPending

  function handleSave() {
    if (!canSave) return
    // Built as a variable (not an inline literal) so `category` and
    // `memory_mission` — not yet in the generated OpenAPI schema — pass the
    // type check. memory_mission is sent only when the user actually changed
    // it from the effective value, so a displayed category default is never
    // silently persisted by unrelated edits (the channel keeps following the
    // default, including future template updates).
    const body = {
      name: name.trim(),
      description,
      default_agent_id: agentId,
      visibility,
      ...(memoryMission !== missionBaseline
        ? { memory_mission: memoryMission }
        : {}),
      ...(category ? { category } : {}),
    }
    updateChannel.mutate(
      {
        params: { path: { id } },
        body,
      },
      {
        onSuccess: () => {
          toast.success("Channel updated")
          queryClient.invalidateQueries({
            queryKey: ["get", "/v1/channels/{id}"],
          })
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/channels"] })
        },
        onError: (error) =>
          toast.danger(extractErrorMessage(error, "Could not update channel")),
      }
    )
  }

  function handleDelete() {
    if (deleteChannel.isPending) return
    deleteChannel.mutate(
      { params: { path: { id } } },
      {
        onSuccess: () => {
          toast.success("Channel deleted")
          setConfirmOpen(false)
          queryClient.invalidateQueries({ queryKey: ["get", "/v1/channels"] })
          router.push("/w/settings/channels")
        },
        onError: (error) =>
          toast.danger(extractErrorMessage(error, "Could not delete channel")),
      }
    )
  }

  if (channelQuery.isLoading) {
    return (
      <div className="flex flex-col gap-8">
        <BackLink />
        <div className="flex flex-col gap-2">
          <Skeleton className="h-8 w-48 rounded" />
          <Skeleton className="h-4 w-72 rounded" />
        </div>
        <Skeleton className="h-56 rounded-2xl" />
      </div>
    )
  }

  if (!channel) {
    return (
      <div className="flex flex-col gap-6">
        <BackLink />
        <section className="bg-surface rounded-2xl border border-border px-4 py-8 text-center">
          <h1 className="text-sm font-medium text-foreground">
            Channel not found
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            This channel may have been deleted.
          </p>
        </section>
      </div>
    )
  }

  const isProtected = Boolean(channel.is_default) || channel.kind === "personal"

  return (
    <div className="flex flex-col gap-8">
      <BackLink />

      <div className="min-w-0">
        <h1 className="flex items-center gap-2 truncate text-lg font-semibold text-foreground">
          <AppIcon icon="hash" className="h-5 w-5 text-muted-foreground" />
          {channel.name}
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          {channel.member_count ?? members.length}{" "}
          {(channel.member_count ?? members.length) === 1
            ? "member"
            : "members"}
        </p>
      </div>

      <nav aria-label="Channel settings" className="flex items-center gap-1">
        <TabButton
          active={tab === "channel"}
          label="Channel"
          onClick={() => setTab("channel")}
        />
        <TabButton
          active={tab === "members"}
          label={members.length ? `Members ${members.length}` : "Members"}
          onClick={() => setTab("members")}
        />
        <TabButton
          active={tab === "knowledge"}
          label="Knowledge sources"
          onClick={() => setTab("knowledge")}
        />
        <TabButton
          active={tab === "env"}
          label="Environment variables"
          onClick={() => setTab("env")}
        />
      </nav>

      {tab === "channel" ? (
        <ChannelTab
          name={name}
          description={description}
          agentId={agentId}
          visibility={visibility}
          category={category}
          memoryMission={memoryMission}
          missionIsDefault={
            !memorySettings.memoryMission &&
            memoryMission !== "" &&
            memoryMission === memorySettings.defaultMemoryMission
          }
          agents={agents}
          agentsLoading={agentsQuery.isLoading}
          isProtected={isProtected}
          canSave={canSave}
          saving={updateChannel.isPending}
          deleting={deleteChannel.isPending}
          onName={setName}
          onDescription={setDescription}
          onAgent={setAgentId}
          onVisibility={setVisibility}
          onCategory={setCategory}
          onMemoryMission={setMemoryMission}
          onSave={handleSave}
          onDelete={() => setConfirmOpen(true)}
        />
      ) : tab === "members" ? (
        <MembersTab members={members} />
      ) : tab === "knowledge" ? (
        <KnowledgeSourcesTab channelId={id} />
      ) : (
        <ChannelEnvironmentVariablesPanel channelId={id} />
      )}

      <ConfirmDialog
        open={confirmOpen}
        pending={deleteChannel.isPending}
        heading={`Delete #${channel.name ?? "channel"}`}
        description="This archives all of the channel's sessions and permanently deletes its memories. This cannot be undone."
        confirmLabel="Delete channel"
        onOpenChange={setConfirmOpen}
        onConfirm={handleDelete}
      />
    </div>
  )
}

function ChannelTab({
  name,
  description,
  agentId,
  visibility,
  category,
  memoryMission,
  missionIsDefault,
  agents,
  agentsLoading,
  isProtected,
  canSave,
  saving,
  deleting,
  onName,
  onDescription,
  onAgent,
  onVisibility,
  onCategory,
  onMemoryMission,
  onSave,
  onDelete,
}: {
  name: string
  description: string
  agentId: string
  visibility: string
  category: string
  memoryMission: string
  missionIsDefault: boolean
  agents: SidebarAgentResponse[]
  agentsLoading: boolean
  isProtected: boolean
  canSave: boolean
  saving: boolean
  deleting: boolean
  onName: (value: string) => void
  onDescription: (value: string) => void
  onAgent: (value: string) => void
  onVisibility: (value: string) => void
  onCategory: (value: string) => void
  onMemoryMission: (value: string) => void
  onSave: () => void
  onDelete: () => void
}) {
  return (
    <div className="flex flex-col gap-6">
      <FormSection title="Name" description="The channel's display name.">
        <Input
          aria-label="Channel name"
          value={name}
          onChange={(event) => onName(event.target.value)}
          className="h-9 w-full rounded-md"
        />
      </FormSection>

      <FormSection title="Description" description="What this channel is for.">
        <Input
          aria-label="Channel description"
          value={description}
          onChange={(event) => onDescription(event.target.value)}
          placeholder="Optional"
          className="h-9 w-full rounded-md"
        />
      </FormSection>

      <FormSection
        title="Assigned agent"
        description="Handles mentions in this channel."
      >
        <AgentSelect
          agents={agents}
          selectedAgentID={agentId}
          isLoading={agentsLoading}
          onChange={onAgent}
          variant="field"
        />
      </FormSection>

      <div className="flex items-center justify-between gap-4">
        <div className="flex flex-col gap-1">
          <h2 className="text-sm font-semibold text-foreground">Private</h2>
          <p className="text-muted-foreground text-sm leading-5">
            Private channels are visible only to invited members.
          </p>
        </div>
        <Switch
          aria-label="Private channel"
          isSelected={visibility === "private"}
          onChange={(selected) => onVisibility(selected ? "private" : "public")}
          className="shrink-0"
        >
          <Switch.Control>
            <Switch.Thumb />
          </Switch.Control>
        </Switch>
      </div>

      <FormSection
        title="Category"
        description={
          CHANNEL_CATEGORIES.find((option) => option.value === category)
            ?.description ?? "Sets what this channel's agent remembers."
        }
      >
        <Select
          aria-label="Memory category"
          selectedKey={category || null}
          onSelectionChange={(key) =>
            onCategory(key === null ? "" : String(key))
          }
          className="w-full"
        >
          <Select.Trigger className="h-9 w-full justify-between px-3 text-sm">
            <span className="truncate">
              {CHANNEL_CATEGORIES.find((option) => option.value === category)
                ?.label ?? "Select category"}
            </span>
            <Select.Indicator />
          </Select.Trigger>
          <Select.Popover className="p-1.5">
            <ListBox>
              {CHANNEL_CATEGORIES.map((option) => (
                <ListBox.Item
                  key={option.value}
                  id={option.value}
                  textValue={option.label}
                >
                  {option.label}
                </ListBox.Item>
              ))}
            </ListBox>
          </Select.Popover>
        </Select>
      </FormSection>

      <FormSection
        title="Memory mission"
        description={
          missionIsDefault
            ? "This channel follows the default mission for its category, shown below. Edit to customize — clearing it returns to the default."
            : "Steers what this channel's agent remembers. Takes priority over default rules."
        }
      >
        <TextArea
          aria-label="Memory mission"
          value={memoryMission}
          onChange={(event) => onMemoryMission(event.target.value)}
          placeholder="e.g. Retain everything durable about this client: preferences, commitments, key contacts."
          rows={3}
          fullWidth
        />
      </FormSection>

      <div className="flex items-center justify-between gap-2">
        {!isProtected ? (
          <Button
            variant="danger-soft"
            size="sm"
            isDisabled={deleting}
            onPress={onDelete}
          >
            {deleting ? (
              <Spinner color="current" size="sm" />
            ) : (
              <AppIcon icon="trash-2" className="h-4 w-4" />
            )}
            Delete channel
          </Button>
        ) : (
          <span />
        )}
        <Button
          variant="primary"
          size="sm"
          isDisabled={!canSave}
          onPress={onSave}
        >
          {saving ? <Spinner color="current" size="sm" /> : null}
          Save changes
        </Button>
      </div>
    </div>
  )
}

function MembersTab({ members }: { members: ChannelMember[] }) {
  return (
    <div className="bg-surface overflow-hidden rounded-2xl border border-border">
      {members.length === 0 ? (
        <div className="px-4 py-8 text-center text-sm text-muted-foreground">
          No members in this channel yet.
        </div>
      ) : (
        members.map((member, index) => (
          <MemberRow
            key={member.user_id ?? index}
            member={member}
            last={index === members.length - 1}
          />
        ))
      )}
    </div>
  )
}

function MemberRow({
  member,
  last,
}: {
  member: ChannelMember
  last: boolean
}) {
  const label = member.name || member.email || "Unknown"
  return (
    <div
      className={`flex items-center gap-3 px-4 py-3.5 ${last ? "" : "border-b border-border"}`}
    >
      <div className="bg-default flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-xs font-medium text-muted-foreground">
        {initials(member.name, member.email)}
      </div>
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="truncate text-sm font-medium text-foreground">
          {label}
        </span>
        <span className="truncate text-sm text-muted-foreground">
          {member.email}
        </span>
      </div>
      <span className="shrink-0 rounded-md bg-default px-1.5 py-0.5 text-xs text-muted-foreground capitalize">
        {member.role}
      </span>
    </div>
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
      aria-current={active ? "page" : undefined}
      onClick={onClick}
      className={`rounded-lg px-3 py-1.5 text-sm font-medium transition-colors ${
        active
          ? "bg-default text-foreground"
          : "text-muted-foreground hover:bg-default/60 hover:text-foreground"
      }`}
    >
      {label}
    </button>
  )
}

function initials(name?: string, email?: string): string {
  const source = (name || email || "?").trim()
  const parts = source.split(/\s+/)
  if (parts.length >= 2) {
    return (parts[0][0] + parts[1][0]).toUpperCase()
  }
  return source.slice(0, 2).toUpperCase()
}

function BackLink() {
  return (
    <NextLink
      href="/w/settings/channels"
      className="inline-flex w-fit items-center gap-2 text-sm text-muted-foreground transition-colors hover:text-foreground"
    >
      <AppIcon icon="arrow-left" className="h-4 w-4" />
      Channels
    </NextLink>
  )
}
