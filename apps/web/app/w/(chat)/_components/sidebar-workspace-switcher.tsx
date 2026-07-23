"use client"

import { useState } from "react"
import {
  Avatar,
  Button,
  Input,
  Modal,
  Popover,
  Spinner,
  toast,
} from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { useAuth } from "@/lib/auth/auth-context"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import {
  labelInitials,
  normalizedWorkspaceName,
  userPrimaryLabel,
  userSecondaryLabel,
} from "@/app/w/(chat)/_lib/workspace-switcher"

export function WorkspaceSwitcher() {
  const [open, setOpen] = useState(false)
  const [createOpen, setCreateOpen] = useState(false)
  const [loggingOut, setLoggingOut] = useState(false)
  const [switchingOrgId, setSwitchingOrgId] = useState<string | null>(null)
  const { user, orgs, activeOrg, setActiveOrg, logout } = useAuth()
  const primaryLabel = userPrimaryLabel(user)
  const secondaryLabel = userSecondaryLabel(user)

  const switchOrg = async (org: (typeof orgs)[number]) => {
    setOpen(false)
    if (!org.id || org.id === activeOrg?.id || switchingOrgId) return
    setSwitchingOrgId(org.id)
    try {
      await setActiveOrg(org)
    } finally {
      setSwitchingOrgId(null)
    }
  }

  const handleLogout = async () => {
    setOpen(false)
    setLoggingOut(true)
    await logout()
  }

  return (
    <>
      <Popover isOpen={open} onOpenChange={setOpen}>
        <Popover.Trigger
          aria-label={`Switch workspace for ${primaryLabel}`}
          className="flex w-full min-w-0 items-center gap-2.5 rounded-xl border border-border bg-background p-2 text-left shadow-sm transition-colors hover:bg-default"
        >
          <Avatar size="sm" className="h-8 w-8 shrink-0">
            {user?.avatar_url ? (
              <Avatar.Image src={user.avatar_url} alt="" />
            ) : null}
            <Avatar.Fallback className="text-xs">
              {labelInitials(primaryLabel)}
            </Avatar.Fallback>
          </Avatar>
          <span className="flex min-w-0 flex-1 flex-col">
            <span className="truncate text-sm font-medium">{primaryLabel}</span>
            {secondaryLabel ? (
              <span className="truncate text-xs text-muted">
                {secondaryLabel}
              </span>
            ) : null}
          </span>
          <AppIcon
            icon="chevron-up"
            className={`h-3.5 w-3.5 shrink-0 text-muted transition-transform duration-150 ease-out ${
              open ? "" : "rotate-180"
            }`}
          />
        </Popover.Trigger>
        <Popover.Content
          placement="top start"
          className="border border-border p-1.5"
          style={{ width: "var(--trigger-width)" }}
        >
          <Popover.Dialog className="flex w-full flex-col gap-0.5 p-0">
            <Popover.Heading className="px-2.5 pt-1.5 pb-1 text-[11px] font-medium tracking-wide text-muted uppercase">
              Workspaces
            </Popover.Heading>
            {orgs.map((org) => {
              const name = org.name?.trim() || "Workspace"
              const selected = org.id === activeOrg?.id
              return (
                <button
                  key={org.id}
                  type="button"
                  disabled={Boolean(switchingOrgId)}
                  onClick={() => void switchOrg(org)}
                  className="flex w-full items-center gap-2.5 rounded-xl px-2.5 py-2 text-left transition-colors hover:bg-default"
                >
                  <Avatar size="sm" className="h-8 w-8 shrink-0">
                    {org.logo_url ? (
                      <Avatar.Image src={org.logo_url} alt="" />
                    ) : null}
                    <Avatar.Fallback className="text-xs">
                      {labelInitials(name)}
                    </Avatar.Fallback>
                  </Avatar>
                  <span className="flex min-w-0 flex-1 flex-col">
                    <span className="truncate text-sm font-medium">{name}</span>
                    {org.role ? (
                      <span className="truncate text-xs text-muted capitalize">
                        {org.role}
                      </span>
                    ) : null}
                  </span>
                  {switchingOrgId === org.id ? (
                    <Spinner size="sm" />
                  ) : selected ? (
                    <AppIcon
                      icon="check"
                      className="h-4 w-4 shrink-0 text-accent"
                    />
                  ) : null}
                </button>
              )
            })}
            <div className="mx-1 my-0.5 border-t border-border" />
            <button
              type="button"
              onClick={() => {
                setOpen(false)
                setCreateOpen(true)
              }}
              className="flex w-full items-center gap-2.5 rounded-xl px-2.5 py-2 text-left text-sm transition-colors hover:bg-default"
            >
              <AppIcon icon="plus" className="h-4 w-4 shrink-0 text-muted" />
              <span className="min-w-0 flex-1 truncate">Add workspace</span>
            </button>
            <button
              type="button"
              disabled={loggingOut}
              onClick={handleLogout}
              className="flex w-full items-center gap-2.5 rounded-xl px-2.5 py-2 text-left text-sm transition-colors hover:bg-default disabled:cursor-progress disabled:opacity-60"
            >
              <AppIcon icon="log-out" className="h-4 w-4 shrink-0 text-muted" />
              <span className="min-w-0 flex-1 truncate">
                {loggingOut ? "Logging out..." : "Log out"}
              </span>
            </button>
          </Popover.Dialog>
        </Popover.Content>
      </Popover>
      <CreateWorkspaceModal open={createOpen} onOpenChange={setCreateOpen} />
    </>
  )
}

function CreateWorkspaceModal({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const [name, setName] = useState("")
  const { addOrg } = useAuth()
  const createOrg = $api.useMutation("post", "/v1/orgs")

  function close() {
    if (createOrg.isPending) return
    setName("")
    onOpenChange(false)
  }

  function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const normalizedName = normalizedWorkspaceName(name)
    if (!normalizedName) {
      toast.danger("Enter a workspace name")
      return
    }

    createOrg.mutate(
      { body: { name: normalizedName } },
      {
        onSuccess: async (org) => {
          await addOrg({ ...org, role: "owner" })
          toast.success(`${normalizedName} created`)
          setName("")
          onOpenChange(false)
        },
        onError: (error) =>
          toast.danger(
            extractErrorMessage(error, "Could not create workspace")
          ),
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
        <Modal.Container placement="center" size="sm">
          <Modal.Dialog className="p-8">
            <Modal.CloseTrigger />
            <form onSubmit={submit}>
              <Modal.Header>
                <Modal.Icon className="size-12 bg-default text-foreground">
                  <AppIcon icon="plus" className="h-6 w-6" />
                </Modal.Icon>
                <div className="flex flex-col gap-1">
                  <Modal.Heading>Add workspace</Modal.Heading>
                  <p className="text-sm text-muted">
                    Create a separate workspace for another company or project.
                  </p>
                </div>
              </Modal.Header>
              <Modal.Body>
                <label className="flex flex-col gap-1.5">
                  <span className="text-sm font-medium">Workspace name</span>
                  <Input
                    autoFocus
                    value={name}
                    disabled={createOrg.isPending}
                    placeholder="Acme Operations"
                    onChange={(event) => setName(event.target.value)}
                  />
                </label>
              </Modal.Body>
              <Modal.Footer>
                <Button
                  type="button"
                  variant="tertiary"
                  size="sm"
                  isDisabled={createOrg.isPending}
                  onPress={close}
                >
                  Cancel
                </Button>
                <Button
                  type="submit"
                  variant="primary"
                  size="sm"
                  isDisabled={
                    createOrg.isPending || !normalizedWorkspaceName(name)
                  }
                >
                  {createOrg.isPending ? (
                    <Spinner color="current" size="sm" />
                  ) : null}
                  Add workspace
                </Button>
              </Modal.Footer>
            </form>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}
