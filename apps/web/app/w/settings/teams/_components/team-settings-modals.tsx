"use client"

import { useMemo, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import {
  Button,
  Input,
  ListBox,
  Modal,
  Select,
  Spinner,
  toast,
} from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { queryKeys } from "@/lib/api/query-keys"
import {
  INVITES_KEY,
  MEMBERS_KEY,
  TEAMS_KEY,
  teamLabel,
  type Team,
} from "./team-settings"

const ROLE_OPTIONS = [
  { value: "admin", label: "Admin" },
  { value: "member", label: "Member" },
]

export function InviteMemberModal({
  open,
  onOpenChange,
  orgName,
  teams,
  defaultTeamIds = [],
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  orgName: string | undefined
  teams: Team[]
  defaultTeamIds?: string[]
}) {
  const queryClient = useQueryClient()
  const createInvite = $api.useMutation("post", "/v1/orgs/current/invites")
  const [email, setEmail] = useState("")
  const [role, setRole] = useState("member")
  const [selectedTeamIds, setSelectedTeamIds] =
    useState<string[]>(defaultTeamIds)

  function close() {
    setEmail("")
    setRole("member")
    setSelectedTeamIds(defaultTeamIds)
    onOpenChange(false)
  }

  function toggleTeam(teamID: string) {
    setSelectedTeamIds((current) =>
      current.includes(teamID)
        ? current.filter((id) => id !== teamID)
        : [...current, teamID]
    )
  }

  function handleSubmit() {
    const trimmed = email.trim()
    if (!trimmed) {
      toast.danger("Enter an email address")
      return
    }
    createInvite.mutate(
      {
        body: {
          email: trimmed,
          role,
          team_ids: selectedTeamIds.length > 0 ? selectedTeamIds : undefined,
        },
      },
      {
        onSuccess: () => {
          toast.success(`Invitation sent to ${trimmed}`)
          queryClient.invalidateQueries({ queryKey: INVITES_KEY })
          queryClient.invalidateQueries({ queryKey: MEMBERS_KEY })
          queryClient.invalidateQueries({ queryKey: TEAMS_KEY })
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
                <AppIcon icon="user-plus" className="h-6 w-6" />
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
                <div className="flex flex-col gap-2">
                  <span className="text-sm font-medium">Teams</span>
                  <TeamPicker
                    teams={teams}
                    selectedTeamIds={selectedTeamIds}
                    onToggle={toggleTeam}
                    isDisabled={createInvite.isPending}
                  />
                </div>
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

export function TeamFormModal({
  open,
  onOpenChange,
  initialTeam,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  initialTeam?: Team
}) {
  const queryClient = useQueryClient()
  const createTeam = $api.useMutation("post", "/v1/orgs/current/teams")
  const updateTeam = $api.useMutation("patch", "/v1/orgs/current/teams/{id}")
  const editing = Boolean(initialTeam?.id)
  const busy = createTeam.isPending || updateTeam.isPending
  const [name, setName] = useState(initialTeam?.name ?? "")
  const [description, setDescription] = useState(initialTeam?.description ?? "")

  function close() {
    if (busy) return
    setName(initialTeam?.name ?? "")
    setDescription(initialTeam?.description ?? "")
    onOpenChange(false)
  }

  function handleSubmit() {
    const trimmed = name.trim()
    if (!trimmed) {
      toast.danger("Enter a team name")
      return
    }
    const body = {
      name: trimmed,
      description: description.trim(),
    }

    if (editing && initialTeam?.id) {
      updateTeam.mutate(
        { params: { path: { id: initialTeam.id } }, body },
        {
          onSuccess: () => {
            toast.success("Team updated")
            queryClient.invalidateQueries({ queryKey: TEAMS_KEY })
            queryClient.invalidateQueries({
              queryKey: queryKeys.team(),
            })
            close()
          },
          onError: (err) =>
            toast.danger(extractErrorMessage(err, "Could not update team")),
        }
      )
      return
    }

    createTeam.mutate(
      { body },
      {
        onSuccess: () => {
          toast.success("Team created")
          queryClient.invalidateQueries({ queryKey: TEAMS_KEY })
          close()
        },
        onError: (err) =>
          toast.danger(extractErrorMessage(err, "Could not create team")),
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
              <Modal.Icon className="size-12 bg-default text-foreground">
                <AppIcon icon="users-round" className="h-6 w-6" />
              </Modal.Icon>
              <div className="flex flex-col gap-1">
                <Modal.Heading>
                  {editing ? "Edit team" : "New team"}
                </Modal.Heading>
                <p className="text-sm text-muted">
                  Teams group members and scope shared resources.
                </p>
              </div>
            </Modal.Header>
            <Modal.Body>
              <div className="flex flex-col gap-4">
                <label className="flex flex-col gap-1.5">
                  <span className="text-sm font-medium">Name</span>
                  <Input
                    value={name}
                    onChange={(event) => setName(event.target.value)}
                    placeholder="Engineering"
                    className="w-full"
                    autoFocus
                  />
                </label>
                <label className="flex flex-col gap-1.5">
                  <span className="text-sm font-medium">Description</span>
                  <Input
                    value={description}
                    onChange={(event) => setDescription(event.target.value)}
                    placeholder="People and resources for this group"
                    className="w-full"
                  />
                </label>
              </div>
            </Modal.Body>
            <Modal.Footer>
              <Button
                variant="tertiary"
                size="sm"
                isDisabled={busy}
                onPress={close}
              >
                Cancel
              </Button>
              <Button
                variant="primary"
                size="sm"
                isDisabled={busy || !name.trim()}
                onPress={handleSubmit}
              >
                {busy ? <Spinner color="current" size="sm" /> : null}
                {editing ? "Save changes" : "Create team"}
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

function TeamPicker({
  teams,
  selectedTeamIds,
  onToggle,
  isDisabled,
}: {
  teams: Team[]
  selectedTeamIds: string[]
  onToggle: (teamID: string) => void
  isDisabled: boolean
}) {
  const selectableTeams = useMemo(
    () => teams.filter((team) => Boolean(team.id)),
    [teams]
  )

  if (selectableTeams.length === 0) {
    return (
      <div className="bg-field-background rounded-xl border border-border px-3 py-3 text-sm text-muted">
        No teams yet.
      </div>
    )
  }

  return (
    <div className="bg-field-background flex flex-col overflow-hidden rounded-xl border border-border">
      {selectableTeams.map((team, index) => {
        const selected = Boolean(team.id && selectedTeamIds.includes(team.id))
        return (
          <button
            key={team.id}
            type="button"
            disabled={isDisabled}
            onClick={() => {
              if (team.id) onToggle(team.id)
            }}
            className="flex items-center gap-3 px-3 py-2 text-left text-sm transition-colors hover:bg-default disabled:cursor-not-allowed disabled:opacity-60"
            style={{
              borderBottom:
                index === selectableTeams.length - 1
                  ? undefined
                  : "1px solid var(--border)",
            }}
          >
            <span
              className={
                selected
                  ? "flex h-4 w-4 shrink-0 items-center justify-center rounded border border-accent bg-accent text-accent-foreground"
                  : "flex h-4 w-4 shrink-0 items-center justify-center rounded border border-border"
              }
            >
              {selected ? <AppIcon icon="check" className="h-3 w-3" /> : null}
            </span>
            <span className="min-w-0 flex-1 truncate">{teamLabel(team)}</span>
            <span className="shrink-0 text-xs text-muted">
              {team.member_count ?? 0} members
            </span>
          </button>
        )
      })}
    </div>
  )
}
