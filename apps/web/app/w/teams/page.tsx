"use client"

import { useState } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  UserAdd01Icon,
  Mail01Icon,
  Tick02Icon,
  Time01Icon,
  Delete01Icon,
  Search01Icon,
} from "@hugeicons/core-free-icons"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { $api } from "@/lib/api/hooks"
import { toast } from "sonner"

type MemberStatus = "accepted" | "pending" | "expired"

interface TeamMember {
  id: string
  name: string
  email: string
  role: "owner" | "admin" | "member" | "viewer"
  status: MemberStatus
  invitedAt?: string
  avatarInitials: string
}

function UserAvatar({
  initials,
  size = 40,
}: {
  initials: string
  size?: number
}) {
  return (
    <div
      className="flex shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary font-semibold"
      style={{ width: size, height: size, fontSize: size * 0.32 }}
    >
      {initials}
    </div>
  )
}

function StatusBadge({ status }: { status: MemberStatus }) {
  const config = {
    accepted: {
      label: "Active",
      icon: Tick02Icon,
      className: "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400",
    },
    pending: {
      label: "Pending",
      icon: Time01Icon,
      className: "bg-amber-500/10 text-amber-600 dark:text-amber-400",
    },
    expired: {
      label: "Expired",
      icon: Time01Icon,
      className: "bg-muted text-muted-foreground",
    },
  }
  const { label, icon, className } = config[status]
  return (
    <Badge variant="outline" className={`gap-1 ${className}`}>
      <HugeiconsIcon icon={icon} className="size-3" />
      {label}
    </Badge>
  )
}

function InviteDialog({
  open,
  onOpenChange,
  onInvite,
  isPending,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onInvite: (email: string, role: string) => void
  isPending: boolean
}) {
  const [email, setEmail] = useState("")
  const [role, setRole] = useState("member")

  function handleSubmit() {
    if (!email.trim() || !email.includes("@")) {
      toast.error("Please enter a valid email address")
      return
    }
    onInvite(email.trim(), role)
    setEmail("")
    setRole("member")
    onOpenChange(false)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Invite team member</DialogTitle>
          <DialogDescription>
            Send an invitation to join your workspace.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <div className="space-y-2">
            <label className="text-sm font-medium text-foreground">Email</label>
            <div className="relative">
              <HugeiconsIcon
                icon={Mail01Icon}
                className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
              />
              <Input
                placeholder="colleague@company.com"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                className="pl-9"
                autoFocus
                onKeyDown={(e) => {
                  if (e.key === "Enter") handleSubmit()
                }}
              />
            </div>
          </div>

          <div className="space-y-2">
            <label className="text-sm font-medium text-foreground">Role</label>
            <div className="flex gap-2">
              {(["member", "admin"] as const).map((r) => (
                <button
                  key={r}
                  onClick={() => setRole(r)}
                  className={`flex-1 rounded-md border px-3 py-2 text-sm font-medium transition-colors ${
                    role === r
                      ? "border-primary bg-primary/5 text-primary"
                      : "border-border bg-muted/30 text-muted-foreground hover:bg-muted/50"
                  }`}
                >
                  {r.charAt(0).toUpperCase() + r.slice(1)}
                </button>
              ))}
            </div>
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={handleSubmit} loading={isPending}>
            Send invitation
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function PendingMemberActionsDialog({
  open,
  onOpenChange,
  member,
  onResend,
  onRevoke,
  isResending,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  member: TeamMember | null
  onResend: (id: string) => void
  onRevoke: (id: string) => void
  isResending: boolean
}) {
  const [confirmDeleteOpen, setConfirmDeleteOpen] = useState(false)

  if (!member) return null

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Manage invitation</DialogTitle>
            <DialogDescription>
              Choose an action for{" "}
              <span className="font-medium text-foreground">{member.email}</span>.
            </DialogDescription>
          </DialogHeader>

          <div className="flex items-center gap-3 rounded-lg border border-border bg-muted/30 p-4">
            <UserAvatar initials={member.avatarInitials} size={40} />
            <div className="min-w-0 flex-1">
              <p className="text-sm font-medium text-foreground truncate">
                {member.name}
              </p>
              <p className="text-xs text-muted-foreground truncate">
                {member.email}
              </p>
            </div>
            <StatusBadge status={member.status} />
          </div>

          <div className="flex flex-col gap-2">
            <Button
              variant="outline"
              className="w-full justify-start gap-2"
              onClick={() => {
                onResend(member.id)
                onOpenChange(false)
              }}
              loading={isResending}
            >
              <HugeiconsIcon icon={Mail01Icon} className="size-4" />
              Resend invitation
            </Button>
            <Button
              variant="destructive"
              className="w-full justify-start gap-2"
              onClick={() => setConfirmDeleteOpen(true)}
            >
              <HugeiconsIcon icon={Delete01Icon} className="size-4" />
              Delete invitation
            </Button>
          </div>

          <DialogFooter className="sm:justify-start">
            <Button variant="ghost" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog open={confirmDeleteOpen} onOpenChange={setConfirmDeleteOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete invitation</AlertDialogTitle>
            <AlertDialogDescription>
              Are you sure you want to delete the invitation for{" "}
              <span className="font-medium text-foreground">{member.email}</span>?
              This action cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={() => {
                onRevoke(member.id)
                onOpenChange(false)
                setConfirmDeleteOpen(false)
              }}
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

export default function TeamsPage() {
  const [inviteOpen, setInviteOpen] = useState(false)
  const [search, setSearch] = useState("")
  const [selectedPending, setSelectedPending] = useState<TeamMember | null>(null)
  const [actionsOpen, setActionsOpen] = useState(false)
  const [resendingId, setResendingId] = useState<string | null>(null)

  const membersQuery = $api.useQuery("get", "/v1/orgs/current/members")
  const invitesQuery = $api.useQuery("get", "/v1/orgs/current/invites")

  const createInvite = $api.useMutation("post", "/v1/orgs/current/invites")
  const revokeInvite = $api.useMutation("delete", "/v1/orgs/current/invites/{id}")
  const resendInvite = $api.useMutation("post", "/v1/orgs/current/invites/{id}/resend")

  const members: TeamMember[] = (membersQuery.data?.data ?? []).map((m) => ({
    id: m.user_id ?? "",
    name: m.name ?? "",
    email: m.email ?? "",
    role: (m.role ?? "member") as TeamMember["role"],
    status: "accepted" as const,
    invitedAt: m.joined_at,
    avatarInitials: (m.name ?? m.email ?? "")
      .split(" ")
      .map((n) => n[0])
      .join("")
      .toUpperCase()
      .slice(0, 2),
  }))

  const invites: TeamMember[] = (invitesQuery.data?.data ?? []).map((inv) => ({
    id: inv.id ?? "",
    name: inv.email ?? "",
    email: inv.email ?? "",
    role: (inv.role ?? "member") as TeamMember["role"],
    status: "pending" as const,
    invitedAt: inv.created_at,
    avatarInitials: (inv.email ?? "")
      .split("@")[0]
      .split(/[._]/)
      .map((n) => n[0])
      .join("")
      .toUpperCase()
      .slice(0, 2),
  }))

  const allMembers = [...members, ...invites]

  const filtered = allMembers.filter(
    (m) =>
      !search ||
      m.name.toLowerCase().includes(search.toLowerCase()) ||
      m.email.toLowerCase().includes(search.toLowerCase())
  )

  const accepted = filtered.filter((m) => m.status === "accepted")
  const pending = filtered.filter((m) => m.status === "pending")

  function handleInvite(email: string, role: string) {
    createInvite.mutate(
      { body: { email, role } },
      {
        onSuccess: () => {
          toast.success(`Invitation sent to ${email}`)
          invitesQuery.refetch()
        },
        onError: (err: any) => {
          const msg = err?.response?.data?.error ?? "Failed to send invitation"
          if (msg === "already a member") {
            toast.error(`${email} is already a member of this workspace`)
          } else if (msg === "invite already pending") {
            toast.error(`An invitation for ${email} is already pending`)
          } else {
            toast.error(msg)
          }
        },
      }
    )
  }

  function handleResendInvite(id: string) {
    setResendingId(id)
    resendInvite.mutate(
      { params: { path: { id } } },
      {
        onSuccess: () => {
          toast.success("Invitation resent")
          invitesQuery.refetch()
        },
        onError: (err: any) => {
          const msg = err?.error ?? "Failed to resend invitation"
          toast.error(msg)
        },
        onSettled: () => {
          setResendingId(null)
        },
      }
    )
  }

  function handleRevokeInvite(id: string) {
    revokeInvite.mutate(
      { params: { path: { id } } },
      {
        onSuccess: () => {
          toast.info("Invitation revoked")
          invitesQuery.refetch()
        },
        onError: () => {
          toast.error("Failed to revoke invitation")
        },
      }
    )
  }

  function handlePendingClick(member: TeamMember) {
    setSelectedPending(member)
    setActionsOpen(true)
  }

  const isLoading = membersQuery.isLoading || invitesQuery.isLoading

  return (
    <div className="mx-auto w-full max-w-5xl flex flex-1 flex-col gap-8">
      <div className="flex items-start justify-between">
        <div>
          <h1 className="font-heading text-3xl font-normal tracking-[-0.02em] text-foreground">
            Teams
          </h1>
          <p className="mt-3 text-sm text-muted-foreground">
            Manage your workspace members and pending invitations.
          </p>
        </div>
        <Button
          onClick={() => setInviteOpen(true)}
          loading={createInvite.isPending}
        >
          <HugeiconsIcon icon={UserAdd01Icon} className="mr-2 size-4" />
          Invite member
        </Button>
      </div>

      <div className="relative">
        <HugeiconsIcon
          icon={Search01Icon}
          className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
        />
        <Input
          placeholder="Search members..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="max-w-sm pl-9"
        />
      </div>

      {isLoading ? (
        <div className="flex items-center justify-center py-16">
          <div className="flex flex-col items-center gap-3">
            <div className="size-8 animate-spin rounded-full border-2 border-primary border-t-transparent" />
            <p className="text-sm text-muted-foreground">Loading team members…</p>
          </div>
        </div>
      ) : (
        <>
          {accepted.length > 0 && (
            <div className="space-y-4">
              <h2 className="font-heading text-lg font-medium text-foreground">
                Active members ({accepted.length})
              </h2>
              <div className="rounded-xl border border-border bg-card">
                {accepted.map((member, i) => (
                  <div
                    key={member.id}
                    className={`flex items-center gap-4 px-5 py-4 ${
                      i !== accepted.length - 1 ? "border-b border-border" : ""
                    }`}
                  >
                    <UserAvatar initials={member.avatarInitials} />
                    <div className="min-w-0 flex-1">
                      <p className="text-sm font-medium text-foreground">
                        {member.name}
                      </p>
                      <p className="text-xs text-muted-foreground">
                        {member.email}
                      </p>
                    </div>
                    <Badge variant="outline" className="capitalize">
                      {member.role}
                    </Badge>
                    <StatusBadge status={member.status} />
                  </div>
                ))}
              </div>
            </div>
          )}

          {pending.length > 0 && (
            <div className="space-y-4">
              <h2 className="font-heading text-lg font-medium text-foreground">
                Pending invitations ({pending.length})
              </h2>
              <div className="rounded-xl border border-border bg-card">
                {pending.map((member, i) => (
                  <button
                    key={member.id}
                    onClick={() => handlePendingClick(member)}
                    className={`flex w-full items-center gap-4 px-5 py-4 text-left transition-colors hover:bg-muted/30 ${
                      i !== pending.length - 1 ? "border-b border-border" : ""
                    }`}
                  >
                    <UserAvatar initials={member.avatarInitials} />
                    <div className="min-w-0 flex-1">
                      <p className="text-sm font-medium text-foreground">
                        {member.email}
                      </p>
                      <p className="text-xs text-muted-foreground">
                        Invited {member.invitedAt ? new Date(member.invitedAt).toLocaleDateString() : ""}
                      </p>
                    </div>
                    <Badge variant="outline" className="capitalize">
                      {member.role}
                    </Badge>
                    <StatusBadge status={member.status} />
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      onClick={(e) => {
                        e.stopPropagation()
                        handlePendingClick(member)
                      }}
                    >
                      <HugeiconsIcon icon={Delete01Icon} className="size-4" />
                    </Button>
                  </button>
                ))}
              </div>
            </div>
          )}

          {accepted.length === 0 && pending.length === 0 && !search && (
            <div className="flex flex-col items-center justify-center py-16 text-center">
              <HugeiconsIcon
                icon={UserAdd01Icon}
                className="mb-4 size-12 text-muted-foreground/30"
              />
              <p className="text-sm font-medium text-foreground">No team members yet</p>
              <p className="mt-1 text-sm text-muted-foreground">
                Invite your first team member to get started.
              </p>
              <Button className="mt-4" onClick={() => setInviteOpen(true)}>
                <HugeiconsIcon icon={UserAdd01Icon} className="mr-2 size-4" />
                Invite member
              </Button>
            </div>
          )}

          {filtered.length === 0 && search && (
            <div className="flex flex-col items-center justify-center py-16 text-center">
              <HugeiconsIcon
                icon={Search01Icon}
                className="mb-4 size-12 text-muted-foreground/30"
              />
              <p className="text-sm text-muted-foreground">
                No members match your search.
              </p>
            </div>
          )}
        </>
      )}

      <InviteDialog
        open={inviteOpen}
        onOpenChange={setInviteOpen}
        onInvite={handleInvite}
        isPending={createInvite.isPending}
      />

      <PendingMemberActionsDialog
        open={actionsOpen}
        onOpenChange={setActionsOpen}
        member={selectedPending}
        onResend={handleResendInvite}
        onRevoke={handleRevokeInvite}
        isResending={resendingId === selectedPending?.id}
      />
    </div>
  )
}
