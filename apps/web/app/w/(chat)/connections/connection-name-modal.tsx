"use client"

import { useState } from "react"
import { Button, Input, Label, Modal, Spinner, toast } from "@heroui/react"
import { $api } from "@/lib/api/hooks"
import { extractErrorMessage } from "@/lib/api/error"

export type ConnectionNameTarget = {
  id: string
  kind: "integration" | "database"
  name: string
  needsName: boolean
}

export function ConnectionNameModal({
  target,
  onClose,
  onSaved,
}: {
  target: ConnectionNameTarget
  onClose: () => void
  onSaved: () => void
}) {
  const [name, setName] = useState(target.name)
  const renameIntegration = $api.useMutation(
    "patch",
    "/v1/connections/{id}/name"
  )
  const renameDatabase = $api.useMutation(
    "patch",
    "/v1/database-integrations/{id}/name"
  )

  const pending = renameIntegration.isPending || renameDatabase.isPending

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!name.trim() || pending) return
    try {
      if (target.kind === "database") {
        await renameDatabase.mutateAsync({
          params: { path: { id: target.id } },
          body: { name: name.trim() },
        })
      } else {
        await renameIntegration.mutateAsync({
          params: { path: { id: target.id } },
          body: { name: name.trim() },
        })
      }
      toast.success("Connection renamed")
      onSaved()
      onClose()
    } catch (error) {
      toast.danger(extractErrorMessage(error, "Could not rename connection"))
    }
  }

  return (
    <Modal isOpen onOpenChange={(open) => !open && onClose()}>
      <Modal.Backdrop className="bg-background/80 backdrop-blur-sm">
        <Modal.Container placement="center" size="sm">
          <Modal.Dialog className="p-6">
            <Modal.CloseTrigger />
            <form onSubmit={submit} className="flex flex-col gap-5">
              <Modal.Header>
                <div>
                  <Modal.Heading>
                    {target.needsName
                      ? "Name this connection"
                      : "Rename connection"}
                  </Modal.Heading>
                  <p className="text-muted-foreground mt-1 text-sm">
                    Agents use this name to distinguish multiple connections.
                  </p>
                </div>
              </Modal.Header>
              <Modal.Body>
                <div className="flex flex-col gap-2">
                  <Label htmlFor="connection-name">Connection name</Label>
                  <Input
                    id="connection-name"
                    autoFocus
                    value={name}
                    maxLength={80}
                    onChange={(event) => setName(event.target.value)}
                    placeholder="Reporting database"
                  />
                </div>
              </Modal.Body>
              <Modal.Footer>
                <Button type="button" variant="secondary" onPress={onClose}>
                  {target.needsName ? "Keep generated name" : "Cancel"}
                </Button>
                <Button
                  type="submit"
                  variant="primary"
                  isDisabled={!name.trim() || pending}
                >
                  {pending ? <Spinner size="sm" color="current" /> : null}
                  Save name
                </Button>
              </Modal.Footer>
            </form>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </Modal>
  )
}
