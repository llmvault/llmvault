"use client"

import { useState, type FormEvent } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { Button, Input, Modal, Spinner, toast } from "@heroui/react"
import { AppIcon } from "@/components/icon"
import { extractErrorMessage } from "@/lib/api/error"
import { $api } from "@/lib/api/hooks"
import { patchSessionInChatCaches } from "@/app/w/(chat)/_lib/chat-cache"

export interface RenameSessionTarget {
  id: string
  name: string
}

export function RenameSessionModal({
  session,
  open,
  onOpenChange,
}: {
  session: RenameSessionTarget | null
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const queryClient = useQueryClient()
  const [name, setName] = useState(session?.name ?? "")
  const renameSession = $api.useMutation("patch", "/v1/sessions/{id}")
  const trimmedName = name.trim()
  const currentName = session?.name.trim() ?? ""
  const invalid = open && trimmedName.length === 0
  const unchanged = trimmedName === currentName

  function close() {
    if (renameSession.isPending) return
    onOpenChange(false)
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!session?.id || invalid || renameSession.isPending) return
    if (unchanged) {
      close()
      return
    }
    renameSession.mutate(
      {
        params: { path: { id: session.id } },
        body: { name: trimmedName },
      },
      {
        onSuccess: (response) => {
          if (response.session) {
            patchSessionInChatCaches(queryClient, response.session)
          }
          toast.success("Chat renamed")
          close()
        },
        onError: (error) =>
          toast.danger(extractErrorMessage(error, "Could not rename chat")),
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
                <Modal.Icon className="bg-default size-12 text-foreground">
                  <AppIcon icon="pencil" className="h-6 w-6" />
                </Modal.Icon>
                <div className="flex flex-col gap-1">
                  <Modal.Heading>Rename chat</Modal.Heading>
                  <p className="text-sm text-muted">
                    Choose a name that is easy to scan later.
                  </p>
                </div>
              </Modal.Header>
              <Modal.Body>
                <label className="flex flex-col gap-1.5">
                  <span className="text-sm font-medium">Chat name</span>
                  <Input
                    autoFocus
                    value={name}
                    disabled={renameSession.isPending}
                    aria-invalid={invalid || undefined}
                    aria-describedby={invalid ? "rename-chat-error" : undefined}
                    onChange={(event) => setName(event.target.value)}
                  />
                </label>
                {invalid ? (
                  <p id="rename-chat-error" className="text-sm text-danger">
                    Enter a chat name.
                  </p>
                ) : null}
              </Modal.Body>
              <Modal.Footer>
                <Button
                  type="button"
                  variant="tertiary"
                  size="sm"
                  isDisabled={renameSession.isPending}
                  onPress={close}
                >
                  Cancel
                </Button>
                <Button
                  type="submit"
                  variant="primary"
                  size="sm"
                  isDisabled={invalid || unchanged || renameSession.isPending}
                >
                  {renameSession.isPending ? (
                    <Spinner color="current" size="sm" />
                  ) : null}
                  Save
                </Button>
              </Modal.Footer>
            </form>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}
