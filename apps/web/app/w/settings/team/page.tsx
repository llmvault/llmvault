"use client"

import { useState } from "react"
import {
  Avatar,
  Button,
  Chip,
  Input,
  ListBox,
  Modal,
  Select,
  Spinner,
  toast,
} from "@heroui/react"
import { Icon } from "@iconify/react"
import { useQueryClient } from "@tanstack/react-query"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"
import { useAuth } from "@/lib/auth/auth-context"
import { cn } from "@/lib/utils"
import type { components } from "@/lib/api/schema"

type Member = components["schemas"]["orgMemberResponse"]
type Invite = components["schemas"]["orgInviteResponse"]

const MEMBERS_KEY = ["get", "/v1/orgs/current/members"]
const INVITES_KEY = ["get", "/v1/orgs/current/invites"]

const ROLE_OPTIONS = [
  { value: "admin", label: "Admin" },
  { value: "member", label: "Member" },
  { value: "viewer", label: "Viewer" },
]

function roleLabel(role: string | undefined) {
  if (!role) return "Member"
  return role.charAt(0).toUpperCase() + role.slice(1)
}

function initials(name: string | undefined, email: string | undefined) {
  const source = (name && name.trim()) || email || "?"
  const parts = source.split(/\s+/).filter(Boolean)
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase()
  return source.slice(0, 2).toUpperCase()
}

function formatDate(value: string | undefined | null) {
  if (!value) return "—"
  return new Intl.DateTimeFormat("en-NG", {
    day: "numeric",
    month: "short",
    year: "numeric",
  }).format(new Date(value))
}

export default function TeamSettingsPage() {
  const { user, activeOrg } = useAuth()
  const isAdmin = activeOrg?.role === "owner" || activeOrg?.role === "admin"
  const [inviteOpen, setInviteOpen] = useState(false)

  const membersQuery = $api.useQuery("get", "/v1/orgs/current/members")
  const invitesQuery = $api.useQuery(
    "get",
    "/v1/orgs/current/invites",
    {},
    { enabled: isAdmin }
  )

  const members = (membersQuery.data?.data ?? []) as Member[]
  const invites = (invitesQuery.data?.data ?? []) as Invite[]

  return (
    <div className="flex flex-col gap-10">
      <div className="flex items-start justify-between gap-4">
        <div className="flex flex-col gap-1">
          <h1 className="text-2xl font-semibold">Team members</h1>
          <p className="text-sm text-muted">
            Manage who has access to {activeOrg?.name ?? "your workspace"}.
          </p>
        </div>
        {isAdmin ? (
          <Button variant="primary" size="sm" onPress={() => setInviteOpen(true)}>
            <Icon icon="lucide:plus" className="h-4 w-4" />
            Invite member
          </Button>
        ) : null}
      </div>

      <section className="flex flex-col gap-3">
        <h2 className="text-sm font-medium">
          Members{members.length ? ` · ${members.length}` : ""}
        </h2>
        <div className="overflow-hidden rounded-2xl border border-border bg-surface">
          {membersQuery.isLoading ? (
            <RowSkeleton />
          ) : members.length === 0 ? (
            <EmptyRow text="No members yet." />
          ) : (
            members.map((member, index) => (
              <MemberRow
                key={member.user_id ?? member.email}
                member={member}
                isYou={member.user_id === user?.id}
                last={index === members.length - 1}
              />
            ))
          )}
        </div>
      </section>

      {isAdmin ? (
        <section className="flex flex-col gap-3">
          <h2 className="text-sm font-medium">
            Pending invitations{invites.length ? ` · ${invites.length}` : ""}
          </h2>
          <div className="overflow-hidden rounded-2xl border border-border bg-surface">
            {invitesQuery.isLoading ? (
              <RowSkeleton />
            ) : invites.length === 0 ? (
              <EmptyRow text="No pending invitations." />
            ) : (
              invites.map((invite, index) => (
                <InviteRow
                  key={invite.id}
                  invite={invite}
                  last={index === invites.length - 1}
                />
              ))
            )}
          </div>
        </section>
      ) : null}

      <InviteModal
        open={inviteOpen}
        onOpenChange={setInviteOpen}
        orgName={activeOrg?.name}
      />
    </div>
  )
}

function MemberRow({
  member,
  isYou,
  last,
}: {
  member: Member
  isYou: boolean
  last?: boolean
}) {
  return (
    <div
      className={cn(
        "flex items-center gap-3 px-4 py-3.5",
        last ? "" : "border-b border-border"
      )}
    >
      <Avatar size="sm" className="shrink-0">
        <Avatar.Fallback>{initials(member.name, member.email)}</Avatar.Fallback>
      </Avatar>
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="truncate text-sm font-medium">
          {member.name || member.email}
          {isYou ? <span className="text-muted"> (You)</span> : null}
        </span>
        <span className="truncate text-sm text-muted">{member.email}</span>
      </div>
      <Chip
        size="sm"
        color={member.role === "owner" ? "accent" : "default"}
      >
        {roleLabel(member.role)}
      </Chip>
    </div>
  )
}

function InviteRow({ invite, last }: { invite: Invite; last?: boolean }) {
  const queryClient = useQueryClient()
  const revoke = $api.useMutation("delete", "/v1/orgs/current/invites/{id}")
  const resend = $api.useMutation("post", "/v1/orgs/current/invites/{id}/resend")
  const busy = revoke.isPending || resend.isPending

  function handleResend() {
    if (!invite.id) return
    resend.mutate(
      { params: { path: { id: invite.id } } },
      {
        onSuccess: () => toast.success(`Invitation resent to ${invite.email}`),
        onError: (err) =>
          toast.danger(extractErrorMessage(err, "Could not resend invitation")),
      }
    )
  }

  function handleRevoke() {
    if (!invite.id) return
    revoke.mutate(
      { params: { path: { id: invite.id } } },
      {
        onSuccess: () => {
          toast.success(`Revoked invitation for ${invite.email}`)
          queryClient.invalidateQueries({ queryKey: INVITES_KEY })
        },
        onError: (err) =>
          toast.danger(extractErrorMessage(err, "Could not revoke invitation")),
      }
    )
  }

  return (
    <div
      className={cn(
        "flex items-center gap-3 px-4 py-3.5",
        last ? "" : "border-b border-border"
      )}
    >
      <Avatar size="sm" className="shrink-0">
        <Avatar.Fallback>
          <Icon icon="lucide:mail" className="h-4 w-4" />
        </Avatar.Fallback>
      </Avatar>
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="truncate text-sm font-medium">{invite.email}</span>
        <span className="truncate text-sm text-muted">
          Invited {formatDate(invite.created_at)} · expires{" "}
          {formatDate(invite.expires_at)}
        </span>
      </div>
      <Chip size="sm">{roleLabel(invite.role)}</Chip>
      <div className="flex shrink-0 items-center gap-1">
        <Button
          variant="ghost"
          size="sm"
          isDisabled={busy}
          onPress={handleResend}
        >
          {resend.isPending ? <Spinner color="current" size="sm" /> : null}
          Resend
        </Button>
        <Button
          variant="ghost"
          size="sm"
          className="text-danger"
          isDisabled={busy}
          onPress={handleRevoke}
        >
          {revoke.isPending ? <Spinner color="current" size="sm" /> : null}
          Revoke
        </Button>
      </div>
    </div>
  )
}

function InviteModal({
  open,
  onOpenChange,
  orgName,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  orgName: string | undefined
}) {
  const queryClient = useQueryClient()
  const createInvite = $api.useMutation("post", "/v1/orgs/current/invites")
  const [email, setEmail] = useState("")
  const [role, setRole] = useState("member")

  function close() {
    setEmail("")
    setRole("member")
    onOpenChange(false)
  }

  function handleSubmit() {
    const trimmed = email.trim()
    if (!trimmed) {
      toast.danger("Enter an email address")
      return
    }
    createInvite.mutate(
      { body: { email: trimmed, role } },
      {
        onSuccess: () => {
          toast.success(`Invitation sent to ${trimmed}`)
          queryClient.invalidateQueries({ queryKey: INVITES_KEY })
          queryClient.invalidateQueries({ queryKey: MEMBERS_KEY })
          close()
        },
        onError: (err) =>
          toast.danger(extractErrorMessage(err, "Could not send invitation")),
      }
    )
  }

  return (
    <Modal
      isOpen={open}
      onOpenChange={(next) => {
        if (!next) {
          if (!createInvite.isPending) close()
        } else {
          onOpenChange(true)
        }
      }}
    >
      <Modal.Backdrop className="bg-background/80 backdrop-blur-sm">
        <Modal.Container placement="center" size="md">
          <Modal.Dialog className="p-8">
            <Modal.CloseTrigger />

            <Modal.Header>
              <Modal.Icon className="size-12 bg-default text-foreground">
                <Icon icon="lucide:user-plus" className="h-6 w-6" />
              </Modal.Icon>
              <div className="flex flex-col gap-1">
                <Modal.Heading>Invite a member</Modal.Heading>
                <p className="text-sm text-muted">
                  They&apos;ll get an email invitation to join{" "}
                  {orgName ?? "your workspace"}.
                </p>
              </div>
            </Modal.Header>

            <Modal.Body>
              <div className="flex flex-col gap-4">
                <label className="flex flex-col gap-1.5">
                  <span className="text-sm font-medium">Email address</span>
                  <Input
                    type="email"
                    value={email}
                    onChange={(event) => setEmail(event.target.value)}
                    placeholder="teammate@company.com"
                    className="w-full"
                    autoFocus
                  />
                </label>
                <label className="flex flex-col gap-1.5">
                  <span className="text-sm font-medium">Role</span>
                  <RoleSelect value={role} onChange={setRole} />
                </label>
              </div>
            </Modal.Body>

            <Modal.Footer>
              <Button
                variant="tertiary"
                size="sm"
                isDisabled={createInvite.isPending}
                onPress={close}
              >
                Cancel
              </Button>
              <Button
                variant="primary"
                size="sm"
                isDisabled={createInvite.isPending || !email.trim()}
                onPress={handleSubmit}
              >
                {createInvite.isPending ? (
                  <Spinner color="current" size="sm" />
                ) : null}
                Send invitation
              </Button>
            </Modal.Footer>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}

function RoleSelect({
  value,
  onChange,
}: {
  value: string
  onChange: (value: string) => void
}) {
  return (
    <Select
      aria-label="Role"
      selectedKey={value}
      onSelectionChange={(key) => onChange(String(key))}
      className="w-full"
    >
      <Select.Trigger className="w-full justify-between">
        <Select.Value />
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover>
        <ListBox>
          {ROLE_OPTIONS.map((option) => (
            <ListBox.Item key={option.value} id={option.value}>
              {option.label}
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}

function EmptyRow({ text }: { text: string }) {
  return <div className="px-4 py-8 text-center text-sm text-muted">{text}</div>
}

function RowSkeleton() {
  return (
    <div className="flex flex-col">
      {[0, 1].map((index) => (
        <div
          key={index}
          className={cn(
            "flex items-center gap-3 px-4 py-3.5",
            index === 1 ? "" : "border-b border-border"
          )}
        >
          <div className="h-9 w-9 shrink-0 animate-pulse rounded-full bg-default" />
          <div className="flex flex-1 flex-col gap-1.5">
            <div className="h-3.5 w-32 animate-pulse rounded bg-default" />
            <div className="h-3 w-48 animate-pulse rounded bg-default" />
          </div>
        </div>
      ))}
    </div>
  )
}
