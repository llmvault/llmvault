"use client"

import { useMemo, useState, type Key } from "react"
import { useQueryClient } from "@tanstack/react-query"
import {
  Avatar,
  Button,
  Chip,
  ComboBox,
  Input,
  ListBox,
  Modal,
  Spinner,
  toast,
} from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import type { components } from "@/lib/api/schema"
import {
  CHAT_QUERY_STALE_TIME_MS,
  chatQueryKeys,
  invalidateSessionListQueries,
} from "@/app/w/(chat)/_lib/chat-cache"

type OrgMember = components["schemas"]["orgMemberResponse"]

export function ShareSessionModal({
  sessionId,
  open,
  onOpenChange,
}: {
  sessionId: string | null
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const queryClient = useQueryClient()
  const [selectedIDs, setSelectedIDs] = useState<Set<string>>(new Set())
  const [inputValue, setInputValue] = useState("")
  const effectiveSessionID = sessionId ?? ""
  const membersQuery = $api.useQuery(
    "get",
    "/v1/orgs/current/members",
    {},
    { enabled: open, retry: false, staleTime: CHAT_QUERY_STALE_TIME_MS }
  )
  const sessionQuery = $api.useQuery(
    "get",
    "/v1/sessions/{id}",
    { params: { path: { id: effectiveSessionID } } },
    {
      enabled: open && Boolean(sessionId),
      retry: false,
      staleTime: CHAT_QUERY_STALE_TIME_MS,
    }
  )
  const shareParticipants = $api.useMutation(
    "post",
    "/v1/sessions/{id}/participants"
  )
  const members = useMemo(
    () => membersQuery.data?.data ?? [],
    [membersQuery.data?.data]
  )
  const membersByID = useMemo(
    () =>
      new Map(
        members.flatMap((member) =>
          member.user_id ? ([[member.user_id, member]] as const) : []
        )
      ),
    [members]
  )
  const participantIDs = useMemo(
    () =>
      new Set(
        (sessionQuery.data?.participants ?? []).flatMap((participant) =>
          participant.user_id ? [participant.user_id] : []
        )
      ),
    [sessionQuery.data?.participants]
  )
  const filteredMembers = useMemo(() => {
    const query = inputValue.trim().toLowerCase()
    if (!query) return members
    return members.filter((member) =>
      memberSearchValue(member).toLowerCase().includes(query)
    )
  }, [inputValue, members])
  const selectedMembers = useMemo(
    () =>
      Array.from(selectedIDs)
        .map((id) => membersByID.get(id))
        .filter((member): member is OrgMember => Boolean(member)),
    [membersByID, selectedIDs]
  )
  const loading = membersQuery.isLoading || sessionQuery.isLoading
  const busy = loading || shareParticipants.isPending

  function close() {
    if (shareParticipants.isPending) return
    setSelectedIDs(new Set())
    setInputValue("")
    onOpenChange(false)
  }

  function selectMember(key: Key | null) {
    if (key == null) return
    const id = String(key)
    if (!membersByID.has(id) || participantIDs.has(id) || selectedIDs.has(id)) {
      return
    }
    setSelectedIDs((current) => new Set(current).add(id))
    setInputValue("")
  }

  function removeMember(id: string) {
    setSelectedIDs((current) => {
      const next = new Set(current)
      next.delete(id)
      return next
    })
  }

  function share() {
    if (!sessionId || selectedIDs.size === 0) return
    const userIDs = Array.from(selectedIDs)
    shareParticipants.mutate(
      {
        params: { path: { id: sessionId } },
        body: { user_ids: userIDs },
      },
      {
        onSuccess: (response) => {
          queryClient.setQueryData(chatQueryKeys.session(sessionId), response)
          invalidateSessionListQueries(queryClient)
          toast.success(shareToast(userIDs.length))
          close()
        },
        onError: (error) =>
          toast.danger(extractErrorMessage(error, "Could not share chat")),
      }
    )
  }

  return (
    <Modal
      isOpen={open}
      onOpenChange={(next) => {
        if (!next) close()
        else onOpenChange(true)
      }}
    >
      <Modal.Backdrop className="bg-background/80 backdrop-blur-sm">
        <Modal.Container placement="center" size="md">
          <Modal.Dialog className="p-8">
            <Modal.CloseTrigger />
            <Modal.Header>
              <Modal.Icon className="bg-default size-12 text-foreground">
                <AppIcon icon="share" className="h-6 w-6" />
              </Modal.Icon>
              <div className="flex flex-col gap-1">
                <Modal.Heading>Share chat</Modal.Heading>
                <p className="text-sm text-muted">
                  Add teammates as participants in this session.
                </p>
              </div>
            </Modal.Header>
            <Modal.Body>
              <div className="flex flex-col gap-4">
                {selectedMembers.length ? (
                  <div className="flex flex-wrap gap-2">
                    {selectedMembers.map((member) => (
                      <SelectedMemberChip
                        key={member.user_id}
                        member={member}
                        onRemove={() => removeMember(member.user_id ?? "")}
                      />
                    ))}
                  </div>
                ) : null}
                {loading ? (
                  <div className="flex min-h-32 items-center justify-center rounded-xl border border-border">
                    <Spinner size="sm" />
                  </div>
                ) : (
                  <MemberComboBox
                    members={filteredMembers}
                    participantIDs={participantIDs}
                    selectedIDs={selectedIDs}
                    inputValue={inputValue}
                    onInputChange={setInputValue}
                    onSelect={selectMember}
                    isDisabled={busy}
                  />
                )}
              </div>
            </Modal.Body>
            <Modal.Footer>
              <Button
                variant="tertiary"
                size="sm"
                isDisabled={shareParticipants.isPending}
                onPress={close}
              >
                Cancel
              </Button>
              <Button
                variant="primary"
                size="sm"
                isDisabled={busy || selectedIDs.size === 0}
                onPress={share}
              >
                {shareParticipants.isPending ? (
                  <Spinner color="current" size="sm" />
                ) : null}
                Share
              </Button>
            </Modal.Footer>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}

function MemberComboBox({
  members,
  participantIDs,
  selectedIDs,
  inputValue,
  onInputChange,
  onSelect,
  isDisabled,
}: {
  members: OrgMember[]
  participantIDs: Set<string>
  selectedIDs: Set<string>
  inputValue: string
  onInputChange: (value: string) => void
  onSelect: (key: Key | null) => void
  isDisabled: boolean
}) {
  return (
    <ComboBox<OrgMember>
      fullWidth
      isDisabled={isDisabled}
      inputValue={inputValue}
      items={members}
      menuTrigger="focus"
      selectedKey={null}
      onInputChange={onInputChange}
      onSelectionChange={onSelect}
    >
      <ComboBox.InputGroup className="flex h-10 items-center gap-2 rounded-xl border border-border px-3">
        <AppIcon icon="search" className="h-4 w-4 shrink-0 text-muted" />
        <Input
          aria-label="Search organization members"
          placeholder="Search members"
          className="min-w-0 flex-1 bg-transparent text-sm outline-none"
        />
        <ComboBox.Trigger className="shrink-0 text-muted" />
      </ComboBox.InputGroup>
      <ComboBox.Popover className="max-h-72 rounded-xl p-1.5">
        {members.length ? (
          <ListBox<OrgMember> items={members}>
            {(member) => {
              const id = member.user_id ?? ""
              const alreadyShared = participantIDs.has(id)
              const selected = selectedIDs.has(id)
              return (
                <ListBox.Item
                  key={id}
                  id={id}
                  textValue={memberSearchValue(member)}
                  isDisabled={alreadyShared || selected}
                >
                  <MemberOption
                    member={member}
                    state={
                      alreadyShared
                        ? "Already in chat"
                        : selected
                          ? "Selected"
                          : undefined
                    }
                  />
                </ListBox.Item>
              )
            }}
          </ListBox>
        ) : (
          <div className="px-3 py-6 text-center text-sm text-muted">
            No members found
          </div>
        )}
      </ComboBox.Popover>
    </ComboBox>
  )
}

function MemberOption({
  member,
  state,
}: {
  member: OrgMember
  state?: string
}) {
  return (
    <div className="flex min-w-0 items-center gap-3">
      <MemberAvatar member={member} />
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium">{memberLabel(member)}</p>
        <p className="truncate text-xs text-muted">{member.email}</p>
      </div>
      {state ? (
        <span className="shrink-0 text-xs text-muted">{state}</span>
      ) : null}
    </div>
  )
}

function SelectedMemberChip({
  member,
  onRemove,
}: {
  member: OrgMember
  onRemove: () => void
}) {
  return (
    <Chip size="sm" variant="soft" className="gap-1.5">
      <Chip.Label>{memberLabel(member)}</Chip.Label>
      <button
        type="button"
        aria-label={`Remove ${memberLabel(member)}`}
        className="hover:bg-default flex h-4 w-4 items-center justify-center rounded-full text-muted transition-colors hover:text-foreground"
        onClick={onRemove}
      >
        <AppIcon icon="x" className="h-3 w-3" />
      </button>
    </Chip>
  )
}

function MemberAvatar({ member }: { member: OrgMember }) {
  return (
    <Avatar size="sm" className="shrink-0">
      <Avatar.Fallback>{initials(member)}</Avatar.Fallback>
    </Avatar>
  )
}

function memberLabel(member: OrgMember) {
  return member.name?.trim() || member.email?.trim() || "Unknown member"
}

function memberSearchValue(member: OrgMember) {
  return `${memberLabel(member)} ${member.email ?? ""}`.trim()
}

function initials(member: OrgMember) {
  const source = memberLabel(member)
  const parts = source.split(/\s+/).filter(Boolean)
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase()
  return source.slice(0, 2).toUpperCase()
}

function shareToast(count: number) {
  return count === 1
    ? "Chat shared with 1 member"
    : `Chat shared with ${count} members`
}
