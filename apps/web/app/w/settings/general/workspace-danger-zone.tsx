"use client"

import { useMemo, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { Button, ListBox, Select, Spinner, toast } from "@heroui/react"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { useAuth } from "@/lib/auth/auth-context"
import { MEMBERS_KEY, memberLabel } from "../teams/_components/team-settings"

// WorkspaceDangerZone surfaces owner-only, org-wide destructive actions:
// transfer ownership to another member, and permanently delete the workspace.
// It lives on the workspace (General) settings page — these are org-scoped,
// not team-scoped. The parent renders it only for owners; the backend enforces
// the owner requirement too, so there's no reason to show controls that would
// always 403.
export function WorkspaceDangerZone() {
  const { user } = useAuth()
  const queryClient = useQueryClient()
  const membersQuery = $api.useQuery("get", "/v1/orgs/current/members")
  const transferOwnership = $api.useMutation(
    "post",
    "/v1/orgs/current/transfer-ownership"
  )
  const deleteOrg = $api.useMutation("delete", "/v1/orgs/current")
  const [transferTargetId, setTransferTargetId] = useState("")
  const [transferOpen, setTransferOpen] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)

  const members = useMemo(
    () => membersQuery.data?.data ?? [],
    [membersQuery.data?.data]
  )

  // Exclude the current owner from their own transfer target list.
  const candidates = members.filter(
    (member) => member.user_id && member.user_id !== user?.id
  )
  const transferTarget = candidates.find(
    (member) => member.user_id === transferTargetId
  )

  function handleTransfer() {
    if (!transferTarget?.user_id || transferOwnership.isPending) return
    transferOwnership.mutate(
      { body: { new_owner_user_id: transferTarget.user_id } },
      {
        onSuccess: () => {
          toast.success(
            `Ownership transferred to ${memberLabel(transferTarget)}`
          )
          setTransferTargetId("")
          setTransferOpen(false)
          queryClient.invalidateQueries({ queryKey: MEMBERS_KEY })
        },
        onError: (err) => {
          toast.danger(extractErrorMessage(err, "Could not transfer ownership"))
          setTransferOpen(false)
        },
      }
    )
  }

  function handleDelete() {
    deleteOrg.mutate(undefined, {
      onSuccess: () => {
        toast.success("Workspace deleted")
        setDeleteOpen(false)
        window.location.href = "/"
      },
      onError: (err) => {
        toast.danger(extractErrorMessage(err, "Could not delete workspace"))
        setDeleteOpen(false)
      },
    })
  }

  return (
    <div className="flex flex-col gap-8">
      <div>
        <h2 className="text-sm font-medium text-danger">Danger zone</h2>
        <p className="mt-1 text-sm text-muted">
          Owner-only actions that affect the entire workspace.
        </p>
      </div>

      <section className="flex flex-col gap-3">
        <div className="flex flex-col gap-1">
          <h2 className="text-sm font-semibold text-foreground">
            Transfer ownership
          </h2>
          <p className="text-muted-foreground text-sm leading-5">
            Hand this workspace to another member. They become the owner and you
            become an admin.
          </p>
        </div>
        <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
          <Select
            aria-label="Transfer ownership to"
            selectedKey={transferTargetId || null}
            onSelectionChange={(key) =>
              setTransferTargetId(key === null ? "" : String(key))
            }
            isDisabled={transferOwnership.isPending || candidates.length === 0}
            className="w-full min-w-0 sm:flex-1"
          >
            <Select.Trigger className="h-9 w-full justify-between px-3 text-sm transition-colors">
              {transferTarget ? (
                <span className="truncate">{memberLabel(transferTarget)}</span>
              ) : (
                <Select.Value />
              )}
              <Select.Indicator />
            </Select.Trigger>
            <Select.Popover className="p-1.5">
              <ListBox>
                {candidates.map((member) => (
                  <ListBox.Item
                    key={member.user_id}
                    id={member.user_id}
                    textValue={memberLabel(member)}
                  >
                    <span className="flex min-w-0 flex-col">
                      <span className="truncate text-sm">
                        {memberLabel(member)}
                      </span>
                      <span className="truncate text-xs text-muted">
                        {member.email}
                      </span>
                    </span>
                  </ListBox.Item>
                ))}
              </ListBox>
            </Select.Popover>
          </Select>
          <Button
            variant="danger-soft"
            size="sm"
            isDisabled={!transferTargetId || transferOwnership.isPending}
            onPress={() => setTransferOpen(true)}
          >
            {transferOwnership.isPending ? (
              <Spinner color="current" size="sm" />
            ) : null}
            Transfer
          </Button>
        </div>
      </section>

      <div className="flex items-center justify-between gap-4 rounded-2xl border border-danger/30 bg-surface p-4">
        <div className="min-w-0">
          <p className="text-sm font-medium">Delete workspace</p>
          <p className="text-sm text-muted">
            Permanently deletes this workspace and all of its data. This cannot
            be undone.
          </p>
        </div>
        <Button
          variant="danger-soft"
          size="sm"
          className="shrink-0"
          onPress={() => setDeleteOpen(true)}
        >
          Delete workspace
        </Button>
      </div>

      <ConfirmDialog
        open={transferOpen}
        pending={transferOwnership.isPending}
        heading={
          transferTarget
            ? `Transfer ownership to ${memberLabel(transferTarget)}?`
            : "Transfer ownership?"
        }
        description="They become the workspace owner and you become an admin. Only they will be able to transfer it back."
        confirmLabel="Transfer ownership"
        icon="arrow-left-right"
        onOpenChange={setTransferOpen}
        onConfirm={handleTransfer}
      />

      <ConfirmDialog
        open={deleteOpen}
        pending={deleteOrg.isPending}
        heading="Delete this workspace?"
        description="This permanently deletes the workspace and all of its agents, channels, sessions, and data. This cannot be undone."
        confirmLabel="Delete workspace"
        icon="trash-2"
        onOpenChange={setDeleteOpen}
        onConfirm={handleDelete}
      />
    </div>
  )
}
